// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import (
	"testing"
)

type testStruct struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Age      int    `json:"age" validate:"gte=0,lte=150"`
	Optional string `json:"optional" validate:"omitempty,min=5"`
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   testStruct
		wantErr bool
	}{
		{
			name: "valid struct",
			input: testStruct{
				Email: "test@example.com",
				Name:  "John Doe",
				Age:   25,
			},
			wantErr: false,
		},
		{
			name: "missing required email",
			input: testStruct{
				Name: "John Doe",
				Age:  25,
			},
			wantErr: true,
		},
		{
			name: "invalid email format",
			input: testStruct{
				Email: "not-an-email",
				Name:  "John Doe",
				Age:   25,
			},
			wantErr: true,
		},
		{
			name: "name too short",
			input: testStruct{
				Email: "test@example.com",
				Name:  "J",
				Age:   25,
			},
			wantErr: true,
		},
		{
			name: "negative age",
			input: testStruct{
				Email: "test@example.com",
				Name:  "John Doe",
				Age:   -1,
			},
			wantErr: true,
		},
		{
			name: "optional field valid",
			input: testStruct{
				Email:    "test@example.com",
				Name:     "John Doe",
				Age:      25,
				Optional: "hello world",
			},
			wantErr: false,
		},
		{
			name: "optional field too short",
			input: testStruct{
				Email:    "test@example.com",
				Name:     "John Doe",
				Age:      25,
				Optional: "hi",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVar(t *testing.T) {
	tests := []struct {
		name    string
		field   interface{}
		tag     string
		wantErr bool
	}{
		{
			name:    "valid email",
			field:   "test@example.com",
			tag:     "email",
			wantErr: false,
		},
		{
			name:    "invalid email",
			field:   "not-an-email",
			tag:     "email",
			wantErr: true,
		},
		{
			name:    "valid uuid",
			field:   "550e8400-e29b-41d4-a716-446655440000",
			tag:     "uuid",
			wantErr: false,
		},
		{
			name:    "invalid uuid",
			field:   "not-a-uuid",
			tag:     "uuid",
			wantErr: true,
		},
		{
			name:    "min length valid",
			field:   "hello",
			tag:     "min=3",
			wantErr: false,
		},
		{
			name:    "min length invalid",
			field:   "hi",
			tag:     "min=3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVar(tt.field, tt.tag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVar() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidationErrors_Error(t *testing.T) {
	errs := ValidationErrors{
		{Field: "email", Message: "this field is required"},
		{Field: "name", Message: "must be at least 2 characters"},
	}

	errStr := errs.Error()
	expected := "email: this field is required; name: must be at least 2 characters"

	if errStr != expected {
		t.Errorf("ValidationErrors.Error() = %v, want %v", errStr, expected)
	}
}

func TestValidateTimezone(t *testing.T) {
	type tzStruct struct {
		Timezone string `validate:"timezone"`
	}

	tests := []struct {
		name     string
		timezone string
		wantErr  bool
	}{
		{"valid Europe/Paris", "Europe/Paris", false},
		{"valid UTC", "UTC", false},
		{"valid America/New_York", "America/New_York", false},
		{"invalid timezone", "Invalid/Zone", true},
		{"empty timezone", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tzStruct{Timezone: tt.timezone}
			err := Validate(&s)
			if (err != nil) != tt.wantErr {
				t.Errorf("timezone validation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLocale(t *testing.T) {
	type localeStruct struct {
		Locale string `validate:"locale"`
	}

	tests := []struct {
		name    string
		locale  string
		wantErr bool
	}{
		{"valid fr", "fr", false},
		{"valid en", "en", false},
		{"invalid de", "de", true},
		{"invalid es", "es", true},
		{"empty locale", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := localeStruct{Locale: tt.locale}
			err := Validate(&s)
			if (err != nil) != tt.wantErr {
				t.Errorf("locale validation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
