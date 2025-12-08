// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import (
	"testing"
)

func TestValidateStrongPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     bool
	}{
		{
			name:     "valid strong password",
			password: "MyP@ssw0rd123!",
			want:     true,
		},
		{
			name:     "valid with all special chars",
			password: "Str0ng!P@ss#W0rd$",
			want:     true,
		},
		{
			name:     "too short (11 chars)",
			password: "MyP@ss0rd!",
			want:     false,
		},
		{
			name:     "no uppercase",
			password: "myp@ssw0rd123!",
			want:     false,
		},
		{
			name:     "no lowercase",
			password: "MYP@SSW0RD123!",
			want:     false,
		},
		{
			name:     "no digit",
			password: "MyP@ssword!!!",
			want:     false,
		},
		{
			name:     "no special character",
			password: "MyPassword123",
			want:     false,
		},
		{
			name:     "empty password",
			password: "",
			want:     false,
		},
		{
			name:     "exactly 12 chars valid",
			password: "MyP@ssw0rd1!",
			want:     true,
		},
		{
			name:     "very long valid password",
			password: "ThisIsAVeryLong&SecureP@ssw0rd123!WithManyCharacters",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test struct to validate
			type TestStruct struct {
				Password string `validate:"strongpassword"`
			}

			testData := TestStruct{Password: tt.password}
			err := Validate(&testData)

			if tt.want {
				if err != nil {
					t.Errorf("validateStrongPassword() for %q: expected valid, got error: %v", tt.password, err)
				}
			} else {
				if err == nil {
					t.Errorf("validateStrongPassword() for %q: expected invalid, got valid", tt.password)
				}
			}
		})
	}
}

func TestPasswordValidationMessage(t *testing.T) {
	type TestStruct struct {
		Password string `json:"password" validate:"strongpassword"`
	}

	testData := TestStruct{Password: "weak"}
	err := Validate(&testData)

	if err == nil {
		t.Error("Expected validation error for weak password")
		return
	}

	validationErrs, ok := err.(ValidationErrors)
	if !ok {
		t.Errorf("Expected ValidationErrors, got %T", err)
		return
	}

	if len(validationErrs) != 1 {
		t.Errorf("Expected 1 validation error, got %d", len(validationErrs))
		return
	}

	expectedMessage := "must be at least 12 characters with uppercase, lowercase, number, and special character"
	if validationErrs[0].Message != expectedMessage {
		t.Errorf("Expected message %q, got %q", expectedMessage, validationErrs[0].Message)
	}

	if validationErrs[0].Field != "password" {
		t.Errorf("Expected field 'password', got %q", validationErrs[0].Field)
	}
}
