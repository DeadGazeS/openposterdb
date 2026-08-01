package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type AppError struct {
	Status  int
	Message string
	Err     error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewInvalidIDType(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: msg}
}

func NewIDNotFound(msg string) *AppError {
	return &AppError{Status: http.StatusNotFound, Message: msg}
}

func NewUnauthorized() *AppError {
	return &AppError{Status: http.StatusUnauthorized, Message: "Unauthorized"}
}

func NewForbidden(msg string) *AppError {
	return &AppError{Status: http.StatusForbidden, Message: msg}
}

func NewBadRequest(msg string) *AppError {
	return &AppError{Status: http.StatusBadRequest, Message: msg}
}

func NewAPIError(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "API error", Err: err}
}

func NewIOError(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "IO error", Err: err}
}

func NewImageError(err error) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "image error", Err: err}
}

func NewDBError(msg string) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: "database error: " + msg}
}

func NewOther(msg string) *AppError {
	return &AppError{Status: http.StatusInternalServerError, Message: msg}
}

func (e *AppError) JSON() []byte {
	msg := e.Message
	if e.Status >= 500 {
		msg = "Internal server error"
	}
	b, _ := json.Marshal(map[string]string{"error": msg})
	return b
}
