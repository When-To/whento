// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whento/pkg/validator"
)

func TestJSON(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		data        interface{}
		wantStatus  int
		wantSuccess bool
	}{
		{
			name:        "success response",
			status:      http.StatusOK,
			data:        map[string]string{"message": "hello"},
			wantStatus:  http.StatusOK,
			wantSuccess: true,
		},
		{
			name:        "created response",
			status:      http.StatusCreated,
			data:        map[string]int{"id": 123},
			wantStatus:  http.StatusCreated,
			wantSuccess: true,
		},
		{
			name:        "error status",
			status:      http.StatusBadRequest,
			data:        nil,
			wantStatus:  http.StatusBadRequest,
			wantSuccess: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			JSON(w, tt.status, tt.data)

			if w.Code != tt.wantStatus {
				t.Errorf("JSON() status = %v, want %v", w.Code, tt.wantStatus)
			}

			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("JSON() Content-Type = %v, want application/json", contentType)
			}

			var resp response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if resp.Success != tt.wantSuccess {
				t.Errorf("JSON() success = %v, want %v", resp.Success, tt.wantSuccess)
			}
		})
	}
}

func TestError(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		message string
	}{
		{
			name:    "bad request error",
			status:  http.StatusBadRequest,
			code:    ErrCodeBadRequest,
			message: "Invalid input",
		},
		{
			name:    "not found error",
			status:  http.StatusNotFound,
			code:    ErrCodeNotFound,
			message: "Resource not found",
		},
		{
			name:    "internal error",
			status:  http.StatusInternalServerError,
			code:    ErrCodeInternal,
			message: "Something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			Error(w, tt.status, tt.code, tt.message)

			if w.Code != tt.status {
				t.Errorf("Error() status = %v, want %v", w.Code, tt.status)
			}

			var resp response
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("Failed to unmarshal response: %v", err)
			}

			if resp.Success {
				t.Error("Error() success should be false")
			}

			if resp.Error == nil {
				t.Fatal("Error() error info should not be nil")
			}

			if resp.Error.Code != tt.code {
				t.Errorf("Error() code = %v, want %v", resp.Error.Code, tt.code)
			}

			if resp.Error.Message != tt.message {
				t.Errorf("Error() message = %v, want %v", resp.Error.Message, tt.message)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	errs := validator.ValidationErrors{
		{Field: "email", Message: "required"},
		{Field: "name", Message: "too short"},
	}

	w := httptest.NewRecorder()
	validationError(w, errs)

	if w.Code != http.StatusBadRequest {
		t.Errorf("ValidationError() status = %v, want %v", w.Code, http.StatusBadRequest)
	}

	var resp response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.Success {
		t.Error("ValidationError() success should be false")
	}

	if resp.Error == nil {
		t.Fatal("ValidationError() error info should not be nil")
	}

	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("ValidationError() code = %v, want VALIDATION_ERROR", resp.Error.Code)
	}

	if len(resp.Error.Details) != 2 {
		t.Errorf("ValidationError() details count = %v, want 2", len(resp.Error.Details))
	}
}

func TestValidate(t *testing.T) {
	type input struct {
		Email string `json:"email" validate:"required,email"`
	}

	w := httptest.NewRecorder()
	if !Validate(w, &input{Email: "a@example.com"}) {
		t.Fatal("Validate() rejected a valid input")
	}
	if w.Body.Len() != 0 {
		t.Errorf("Validate() wrote %q for a valid input", w.Body.String())
	}

	w = httptest.NewRecorder()
	if Validate(w, &input{}) {
		t.Fatal("Validate() accepted an invalid input")
	}
	var resp response
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if w.Code != http.StatusBadRequest || resp.Error == nil || resp.Error.Code != ErrCodeValidation || len(resp.Error.Details) != 1 {
		t.Errorf("Validate() wrote status %d, body %s", w.Code, w.Body.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	type testBody struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "valid JSON",
			body:    `{"name": "John", "email": "john@example.com"}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			body:    `{invalid}`,
			wantErr: true,
		},
		{
			name:    "empty body",
			body:    ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var target testBody
			err := DecodeJSON(r, &target)

			if (err != nil) != tt.wantErr {
				t.Errorf("DecodeJSON() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.body != "" {
				if target.Name != "John" {
					t.Errorf("DecodeJSON() name = %v, want John", target.Name)
				}
			}
		})
	}
}
