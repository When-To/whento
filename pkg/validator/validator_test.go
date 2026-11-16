// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import (
	"strings"
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

func TestValidateMaxBytes(t *testing.T) {
	// The unit is *bytes*, because that is the unit bcrypt enforces (72 bytes).
	// 24 ASCII characters fit; 24 CJK characters are 72 bytes and must be
	// refused at the 73rd byte, even though go-playground's `max` (which counts
	// runes) would have accepted both. strings.Repeat keeps the counts exact; a
	// hand-typed row of y's invites an off-by-one that proves nothing.
	ascii72 := strings.Repeat("y", 72)
	ascii73 := strings.Repeat("y", 73)
	cjk24 := strings.Repeat("密", 24) // 3 bytes/char = 72 bytes
	cjk25 := strings.Repeat("密", 25) // 75 bytes

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"ascii within limit", strings.Repeat("y", 24), false},
		{"ascii at the byte limit", ascii72, false},
		{"ascii over the byte limit", ascii73, true},
		{"cjk at the byte limit", cjk24, false},
		{"cjk over the byte limit", cjk25, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Var(tt.value, "maxbytes=72")
			if (err != nil) != tt.wantErr {
				t.Errorf("validate.Var() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
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
