package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// HandleExportSettings returns a JSON settings backup.
//
// POST (used by the SPA) takes {include_service_keys, include_api_keys,
// passphrase} as a JSON body. GET with the same query params remains for
// plain exports, but never accepts a passphrase in the URL (URLs end up in
// proxy/CDN access logs). An export that includes the external provider keys
// must be passphrase-encrypted — otherwise the file holds them in plaintext.
func HandleExportSettings(db *sql.DB, keys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var opts struct {
			IncludeServiceKeys bool   `json:"include_service_keys"`
			IncludeAPIKeys     bool   `json:"include_api_keys"`
			Passphrase         string `json:"passphrase"`
		}
		switch r.Method {
		case http.MethodPost:
			if err := httpx.DecodeJSON(r, &opts); err != nil {
				httpx.WriteError(w, 400, "invalid request body")
				return
			}
		case http.MethodGet:
			q := r.URL.Query()
			if q.Has("passphrase") {
				httpx.WriteError(w, 400, "send the passphrase in a POST body, not the URL")
				return
			}
			opts.IncludeServiceKeys = q.Get("include_service_keys") == "1" || q.Get("include_service_keys") == "true"
			opts.IncludeAPIKeys = q.Get("include_api_keys") == "1" || q.Get("include_api_keys") == "true"
		default:
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		if opts.IncludeServiceKeys && opts.Passphrase == "" {
			httpx.WriteError(w, 400, "a passphrase is required when the export includes the external API keys")
			return
		}

		payload, err := services.BuildExportPayloadCtx(r.Context(), db, keys, opts.IncludeServiceKeys, opts.IncludeAPIKeys)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to export settings")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="openposterdb-settings.json"`)
		w.Header().Set("Cache-Control", "no-store")
		if opts.Passphrase != "" {
			enc, encErr := services.EncryptPayload(payload, opts.Passphrase)
			if encErr != nil {
				slog.Warn("export encryption failed", "error", encErr)
				httpx.WriteError(w, 500, "Failed to encrypt export")
				return
			}
			httpx.WriteJSON(w, 200, enc)
			return
		}
		httpx.WriteJSON(w, 200, payload)
	}
}

// HandleImportSettings restores a settings backup produced by
// HandleExportSettings. The request body can be a plain ExportPayload or an
// EncryptedExport — the kind tag decides the path. API keys that don't exist
// yet are recreated with a new value, which is returned in the response.
func HandleImportSettings(db *sql.DB, keys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}

		// Two accepted body shapes:
		// (a) { "kind": "openposterdb/settings",            ... }                (plain)
		// (b) { "passphrase": "...", "payload": { ... } }                        (encrypted)
		// (c) { "kind": "openposterdb/settings-encrypted", ... }                (encrypted, passphrase in query)
		//
		// Read the body bytes once. httpx.DecodeJSON closes r.Body (its
		// invariant), but resolveImportPayload needs to re-read r.Body when
		// the envelope has no payload field — reading a closed body
		// returns "http: invalid Read on closed Body" (bug observed 2026-08-15
		// docker logs). Pass the bytes through instead.
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.WriteError(w, 400, "could not read request body")
			return
		}
		r.Body.Close()

		var envelope struct {
			Passphrase string          `json:"passphrase"`
			Payload    json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}

		payload, err := resolveImportPayload(envelope, bodyBytes, r)
		if err != nil {
			httpx.WriteError(w, 400, err.Error())
			return
		}

		if payload.Kind != "openposterdb/settings" && payload.Kind != "openposterdb/settings-encrypted" {
			httpx.WriteError(w, 400, "not a settings export file")
			return
		}

		result, err := services.ApplyImportPayloadCtx(r.Context(), db, keys, payload)
		if err != nil {
			slog.Warn("settings import failed", "error", err)
			if result.Committed {
				// Settings and API keys were committed but a later step
				// failed: return what was applied — regenerated key values
				// exist only here.
				httpx.WriteJSON(w, 500, map[string]any{"error": "Settings imported, but service keys failed", "result": result})
				return
			}
			httpx.WriteError(w, 500, "Failed to import settings (nothing was changed)")
			return
		}

		httpx.WriteJSON(w, 200, result)
	}
}

// resolveImportPayload routes the import request to the right decode path.
// Plain (kind=openposterdb/settings) goes straight through; encrypted
// (kind=openposterdb/settings-encrypted) requires a passphrase either in the
// envelope body or in the `passphrase` query string.
//
// bodyBytes is the raw HTTP body, captured once in HandleImportSettings
// before r.Body was closed. We never read r.Body here — doing so would
// trigger "http: invalid Read on closed Body" since httpx.DecodeJSON has
// already closed it by the time we get here.
func resolveImportPayload(envelope struct {
	Passphrase string          `json:"passphrase"`
	Payload    json.RawMessage `json:"payload"`
}, bodyBytes []byte, r *http.Request) (*services.ExportPayload, error) {
	raw := envelope.Payload
	if len(raw) == 0 {
		// No payload field — the body itself is the export. Decode whole body.
		var direct services.ExportPayload
		if err := json.Unmarshal(bodyBytes, &direct); err != nil {
			return nil, err
		}
		if direct.Kind == "openposterdb/settings-encrypted" {
			var enc services.EncryptedExport
			buf, _ := json.Marshal(direct)
			_ = json.Unmarshal(buf, &enc)
			return resolveEncrypted(enc, envelope.Passphrase, r)
		}
		return &direct, nil
	}

	// payload field present — peek at the kind prefix.
	probe := struct {
		Kind string `json:"kind"`
	}{}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if probe.Kind == "openposterdb/settings-encrypted" {
		var enc services.EncryptedExport
		if err := json.Unmarshal(raw, &enc); err != nil {
			return nil, err
		}
		return resolveEncrypted(enc, envelope.Passphrase, r)
	}
	var plain services.ExportPayload
	if err := json.Unmarshal(raw, &plain); err != nil {
		return nil, err
	}
	return &plain, nil
}

func resolveEncrypted(enc services.EncryptedExport, bodyPassphrase string, r *http.Request) (*services.ExportPayload, error) {
	passphrase := bodyPassphrase
	if passphrase == "" {
		passphrase = r.URL.Query().Get("passphrase")
	}
	if passphrase == "" {
		return nil, errors.New("passphrase required for encrypted export")
	}
	return services.DecryptPayload(&enc, passphrase)
}
