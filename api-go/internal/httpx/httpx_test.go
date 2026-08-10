package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperr "openposterdb/internal/errors"
)

// TestWriteJSON_OK_200_WithBody guards the happy path: WriteJSON writes a
// 200 response with a JSON-encoded body and the correct Content-Type.
func TestWriteJSON_OK_200_WithBody(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, 200, map[string]string{"hello": "world"})
	if rec.Code != 200 {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"hello":"world"`) {
		t.Errorf("body missing payload: %s", rec.Body.String())
	}
}

// TestWriteJSON_EncodesCorrectly guards the encoder: a complex value
// (slice of structs) round-trips through WriteJSON correctly.
func TestWriteJSON_EncodesCorrectly(t *testing.T) {
	type item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	rec := httptest.NewRecorder()
	WriteJSON(rec, 201, []item{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}})
	if rec.Code != 201 {
		t.Errorf("status: got %d, want 201", rec.Code)
	}
	var got []item
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 || got[0].ID != 1 || got[1].Name != "b" {
		t.Errorf("decoded: %+v", got)
	}
}

// TestWriteError_400_Body guards the simple-error shape: a 400 + JSON body
// with the error message under the "error" key.
func TestWriteError_400_Body(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusBadRequest, "bad input")
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "bad input" {
		t.Errorf("error: got %q, want %q", body["error"], "bad input")
	}
}

// TestWriteError_500_Body guards the 500 branch: WriteError passes the
// caller-supplied message through verbatim (the caller is responsible for
// sanitisation; WriteAppError does the sanitisation automatically).
func TestWriteError_500_Body(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, http.StatusInternalServerError, "explicit 500 message")
	if rec.Code != 500 {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "explicit 500 message") {
		t.Errorf("body missing message: %s", rec.Body.String())
	}
}

// TestWriteAppError_400 guards the AppError(400) branch: the AppError
// message is exposed verbatim (4xx messages are safe to send to clients).
func TestWriteAppError_400(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAppError(rec, apperr.NewBadRequest("missing field"))
	if rec.Code != 400 {
		t.Errorf("status: got %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing field") {
		t.Errorf("body missing message: %s", rec.Body.String())
	}
}

// TestWriteAppError_500 guards the sanitisation contract: a 5xx AppError
// must never leak the underlying error message to the client — only
// "Internal server error".
func TestWriteAppError_500(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAppError(rec, apperr.NewAPIError(errors.New("SQL: SELECT * FROM secrets")))
	if rec.Code != 500 {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "secrets") || strings.Contains(body, "SQL") {
		t.Errorf("5xx leaked internals: %s", body)
	}
	if !strings.Contains(body, "Internal server error") {
		t.Errorf("5xx body missing sanitised message: %s", body)
	}
}

// TestWriteAppError_404 guards the AppError(404) branch.
func TestWriteAppError_404(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAppError(rec, apperr.NewIDNotFound("title not found"))
	if rec.Code != 404 {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "title not found") {
		t.Errorf("body missing message: %s", rec.Body.String())
	}
}

// TestWriteAppError_503 guards the 5xx-sanitisation contract: any status
// >= 500 (not just 500) gets the AppError message redacted to "Internal
// server error" so internal details never leak.
func TestWriteAppError_503(t *testing.T) {
	rec := httptest.NewRecorder()
	ae := &apperr.AppError{Status: http.StatusServiceUnavailable, Message: "upstream timeout"}
	WriteAppError(rec, ae)
	if rec.Code != 503 {
		t.Errorf("status: got %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "upstream timeout") {
		t.Errorf("503 leaked internals: %s", body)
	}
	if !strings.Contains(body, "Internal server error") {
		t.Errorf("503 body missing sanitised message: %s", body)
	}
}

// TestWriteAppError_Nil guards the nil-error fallback: nil → 500 with
// sanitised "Internal server error" message.
func TestWriteAppError_Nil(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAppError(rec, nil)
	if rec.Code != 500 {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Internal server error") {
		t.Errorf("body missing sanitised message: %s", rec.Body.String())
	}
}

// TestWriteAppError_PlainError guards the non-AppError fallback: a plain
// error is treated as 500 and sanitised.
func TestWriteAppError_PlainError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteAppError(rec, errors.New("raw db failure with sensitive context"))
	if rec.Code != 500 {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "sensitive") {
		t.Errorf("non-AppError leaked internals: %s", body)
	}
}

// TestDecodeJSON_ValidBody guards the happy path: a valid JSON body
// decodes into the destination struct.
func TestDecodeJSON_ValidBody(t *testing.T) {
	body := strings.NewReader(`{"name":"alice","age":30}`)
	req := httptest.NewRequest(http.MethodPost, "/x", body)

	var dst struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	if err := DecodeJSON(req, &dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dst.Name != "alice" || dst.Age != 30 {
		t.Errorf("decoded: %+v", dst)
	}
}

// TestDecodeJSON_InvalidJSON_Error guards the negative path: malformed
// JSON returns an error.
func TestDecodeJSON_InvalidJSON_Error(t *testing.T) {
	body := strings.NewReader(`{not-json`)
	req := httptest.NewRequest(http.MethodPost, "/x", body)

	var dst map[string]any
	if err := DecodeJSON(req, &dst); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestDecodeJSON_EmptyBody_Error guards the empty-body path: no JSON to
// decode → error.
func TestDecodeJSON_EmptyBody_Error(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(""))
	var dst map[string]any
	if err := DecodeJSON(req, &dst); err == nil {
		t.Error("expected error for empty body, got nil")
	}
}

// TestParseIntParam_Valid guards the happy path: a valid integer string is
// parsed and returned.
func TestParseIntParam_Valid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?limit=42", nil)
	if got := ParseIntParam(req, "limit", 10); got != 42 {
		t.Errorf("limit: got %d, want 42", got)
	}
}

// TestParseIntParam_Invalid_400 guards the fallback: an unparsable value
// returns the default.
func TestParseIntParam_Invalid_400(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?limit=notanumber", nil)
	if got := ParseIntParam(req, "limit", 10); got != 10 {
		t.Errorf("limit: got %d, want default 10", got)
	}
}

// TestParseIntParam_NegativeRejected guards the negative-value branch:
// values smaller than 1 fall back to the default (the helper is for
// pagination-style limits that must be >= 1).
func TestParseIntParam_NegativeRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?limit=-5", nil)
	if got := ParseIntParam(req, "limit", 10); got != 10 {
		t.Errorf("limit: got %d, want default 10 (negative rejected)", got)
	}
}

// TestParseIntParam_ZeroRejected guards the zero-value branch: zero is
// not a valid limit, falls back to default.
func TestParseIntParam_ZeroRejected(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x?limit=0", nil)
	if got := ParseIntParam(req, "limit", 10); got != 10 {
		t.Errorf("limit: got %d, want default 10 (zero rejected)", got)
	}
}

// TestParseIntParam_DefaultApplied guards the missing-param branch.
func TestParseIntParam_DefaultApplied(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if got := ParseIntParam(req, "limit", 25); got != 25 {
		t.Errorf("limit: got %d, want default 25", got)
	}
}
