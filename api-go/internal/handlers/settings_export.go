package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"openposterdb/internal/httpx"
	"openposterdb/internal/services"
)

// HandleExportSettings returns a JSON settings backup. Query params
// include_service_keys and include_api_keys control which secrets are included.
// passphrase (optional) wraps the payload in an encrypted envelope so the
// backup file is inert without the user-supplied passphrase.
func HandleExportSettings(db *sql.DB, keys *services.ServiceKeyManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			httpx.WriteError(w, 405, "Method not allowed")
			return
		}
		q := r.URL.Query()
		includeServiceKeys := q.Get("include_service_keys") == "1" || q.Get("include_service_keys") == "true"
		includeAPIKeys := q.Get("include_api_keys") == "1" || q.Get("include_api_keys") == "true"
		passphrase := q.Get("passphrase")

		payload, err := services.BuildExportPayloadCtx(r.Context(), db, keys, includeServiceKeys, includeAPIKeys)
		if err != nil {
			httpx.WriteError(w, 500, "Failed to export settings")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="openposterdb-settings.json"`)
		if passphrase != "" {
			enc, encErr := services.EncryptPayload(payload, passphrase)
			if encErr != nil {
				httpx.WriteError(w, 500, "Failed to encrypt export: "+encErr.Error())
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
		var envelope struct {
			Passphrase string          `json:"passphrase"`
			Payload    json.RawMessage `json:"payload"`
		}
		if err := httpx.DecodeJSON(r, &envelope); err != nil {
			httpx.WriteError(w, 400, "invalid JSON")
			return
		}

		payload, err := resolveImportPayload(envelope, r)
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
			httpx.WriteError(w, 500, "Failed to import settings")
			return
		}

		httpx.WriteJSON(w, 200, result)
	}
}

// resolveImportPayload routes the import request to the right decode path.
// Plain (kind=openposterdb/settings) goes straight through; encrypted
// (kind=openposterdb/settings-encrypted) requires a passphrase either in the
// envelope body or in the `passphrase` query string.
func resolveImportPayload(envelope struct {
	Passphrase string          `json:"passphrase"`
	Payload    json.RawMessage `json:"payload"`
}, r *http.Request) (*services.ExportPayload, error) {
	raw := envelope.Payload
	if len(raw) == 0 {
		// No payload field — the body itself is the export. Re-decode whole body.
		var direct services.ExportPayload
		if err := json.NewDecoder(r.Body).Decode(&direct); err != nil {
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
