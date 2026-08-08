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

func TestBadRequestIs400(t *testing.T) {
	err := NewBadRequest("x")
	if err.Status != 400 {
		t.Errorf("expected 400, got %d", err.Status)
	}
}

func TestOtherIs500(t *testing.T) {
	err := NewOther("something")
	if err.Status != 500 {
		t.Errorf("expected 500, got %d", err.Status)
	}
}

func TestClientMessageRedactsDetailsOn500(t *testing.T) {
	err := NewOther("connection refused to 10.0.0.5:5432")
	if msg := err.ClientMessage(); msg != "Internal server error" {
		t.Errorf("expected 'Internal server error', got %q", msg)
	}
}

func TestClientMessagePreservesMessageOn400(t *testing.T) {
	err := NewBadRequest("missing field")
	if msg := err.ClientMessage(); msg != "missing field" {
		t.Errorf("expected passthrough, got %q", msg)
	}
}
