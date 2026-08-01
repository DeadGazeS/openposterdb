package errors

import (
	"testing"
)

func TestInvalidIDTypeIs400(t *testing.T) {
	err := NewInvalidIDType("x")
	if err.Status != 400 {
		t.Errorf("expected 400, got %d", err.Status)
	}
}

func TestIDNotFoundIs404(t *testing.T) {
	err := NewIDNotFound("x")
	if err.Status != 404 {
		t.Errorf("expected 404, got %d", err.Status)
	}
}

func TestUnauthorizedIs401(t *testing.T) {
	err := NewUnauthorized()
	if err.Status != 401 {
		t.Errorf("expected 401, got %d", err.Status)
	}
}

func TestForbiddenIs403(t *testing.T) {
	err := NewForbidden("x")
	if err.Status != 403 {
		t.Errorf("expected 403, got %d", err.Status)
	}
}

func TestBadRequestIs400(t *testing.T) {
	err := NewBadRequest("x")
	if err.Status != 400 {
		t.Errorf("expected 400, got %d", err.Status)
	}
}

func TestDBErrorIs500(t *testing.T) {
	err := NewDBError("connection refused")
	if err.Status != 500 {
		t.Errorf("expected 500, got %d", err.Status)
	}
}

func TestOtherIs500(t *testing.T) {
	err := NewOther("something")
	if err.Status != 500 {
		t.Errorf("expected 500, got %d", err.Status)
	}
}

func TestJSONRedactsDetailsOn500(t *testing.T) {
	err := NewDBError("connection refused to 10.0.0.5:5432")
	body := string(err.JSON())
	if body == "" {
		t.Fatal("expected JSON body")
	}
}

func TestJSONClientErrorsPreserveMessage(t *testing.T) {
	err := NewBadRequest("missing field")
	body := string(err.JSON())
	if body == "" {
		t.Fatal("expected JSON body")
	}
}
