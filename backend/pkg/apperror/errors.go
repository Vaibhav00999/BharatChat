package apperror

import (
	"errors"
	"net/http"
)

type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
	Err        error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}
func (e *AppError) Unwrap() error { return e.Err }
func New(code, message string, status int, cause error) *AppError {
	return &AppError{Code: code, Message: message, HTTPStatus: status, Err: cause}
}
func NewValidation(m string) *AppError   { return New("VALIDATION_ERROR", m, http.StatusBadRequest, nil) }
func NewNotFound(m string) *AppError     { return New("NOT_FOUND", m, http.StatusNotFound, nil) }
func NewUnauthorized(m string) *AppError { return New("UNAUTHORIZED", m, http.StatusUnauthorized, nil) }
func NewForbidden(m string) *AppError    { return New("FORBIDDEN", m, http.StatusForbidden, nil) }
func NewConflict(m string) *AppError     { return New("CONFLICT", m, http.StatusConflict, nil) }
func NewTooManyRequests(m string) *AppError {
	return New("TOO_MANY_REQUESTS", m, http.StatusTooManyRequests, nil)
}
func NewInternal(err error) *AppError {
	return New("INTERNAL_ERROR", "an unexpected error occurred", http.StatusInternalServerError, err)
}
func As(err error) (*AppError, bool) {
	var a *AppError
	if errors.As(err, &a) {
		return a, true
	}
	return nil, false
}
