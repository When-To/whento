// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package httputil

import (
	"encoding/json"
	"net/http"

	"github.com/whento/pkg/validator"
)

// response is the standard API response envelope
type response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
}

// ErrorInfo represents error details
type ErrorInfo struct {
	Code    string                      `json:"code"`
	Message string                      `json:"message"`
	Details []validator.ValidationError `json:"details,omitempty"`
}

// ErrorResponse represents an API error response (for Swagger documentation)
type ErrorResponse struct {
	Success bool       `json:"success" example:"false"`
	Error   *ErrorInfo `json:"error"`
}

// JSON writes a JSON response with cache-prevention headers
func JSON(w http.ResponseWriter, status int, data interface{}) {
	write(w, status, response{Success: status >= 200 && status < 300, Data: data})
}

// Error writes an error response with cache-prevention headers
func Error(w http.ResponseWriter, status int, code, message string) {
	write(w, status, response{Error: &ErrorInfo{Code: code, Message: message}})
}

// validationError writes a validation error response with cache-prevention headers
func validationError(w http.ResponseWriter, errs validator.ValidationErrors) {
	write(w, http.StatusBadRequest, response{Error: &ErrorInfo{
		Code:    ErrCodeValidation,
		Message: "Invalid input data",
		Details: errs,
	}})
}

// Validate runs struct validation on v and, when it fails, writes the 400
// response. It reports whether v is valid.
func Validate(w http.ResponseWriter, v any) bool {
	err := validator.Validate(v)
	if err == nil {
		return true
	}
	if errs, ok := err.(validator.ValidationErrors); ok {
		validationError(w, errs)
	} else {
		Error(w, http.StatusBadRequest, ErrCodeValidation, err.Error())
	}
	return false
}

func write(w http.ResponseWriter, status int, resp response) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// DecodeJSON decodes JSON request body into target struct
func DecodeJSON(r *http.Request, target interface{}) error {
	return json.NewDecoder(r.Body).Decode(target)
}

// DecodeValid decodes the JSON body into v and validates it, writing the 400
// response on failure. It reports whether the handler may proceed.
func DecodeValid(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := DecodeJSON(r, v); err != nil {
		Error(w, http.StatusBadRequest, ErrCodeBadRequest, "Invalid request body")
		return false
	}
	return Validate(w, v)
}

// Common error codes
const (
	ErrCodeInternal         = "INTERNAL_ERROR"
	ErrCodeBadRequest       = "BAD_REQUEST"
	ErrCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeValidation       = "VALIDATION_ERROR"
	ErrCodeRateLimited      = "RATE_LIMITED"
	ErrCodeUnavailable      = "SERVICE_UNAVAILABLE"
	// ErrCodeBootstrapRequired is the stable machine-readable code for "the
	// instance has no first user yet, so the ordinary auth path is closed"; the
	// client can recognise it and steer the visitor to /bootstrap. Kept as its
	// own code (rather than reusing ErrCodeForbidden) so a client never has to
	// guess which 403 means what.
	ErrCodeBootstrapRequired = "BOOTSTRAP_REQUIRED"
)
