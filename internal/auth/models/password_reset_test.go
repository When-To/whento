// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package models

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestResetPasswordResponseKeepsTheRefreshTokenOutOfJSON pins the one thing the struct
// tag is there for. Reset-password auto-signs the user in, so it carries the same pair
// of tokens Login does — and used to be the only endpoint that serialised the refresh
// token, handing a seven-day credential to anything that could read a response body.
// The handler sets it as an httpOnly cookie; that is the only way it should travel.
func TestResetPasswordResponseKeepsTheRefreshTokenOutOfJSON(t *testing.T) {
	const secret = "a-refresh-token-worth-seven-days"

	resp := &ResetPasswordResponse{
		Message:      "Password reset successfully",
		AccessToken:  "an-access-token",
		RefreshToken: secret,
		User:         &UserResponse{ID: "u-1", Email: "ada@example.test"},
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if strings.Contains(string(encoded), secret) {
		t.Errorf("the refresh token is in the JSON body:\n%s", encoded)
	}
	if strings.Contains(string(encoded), "refresh_token") {
		t.Errorf("the response still carries a refresh_token field:\n%s", encoded)
	}

	// The access token still has to reach the client — it is short-lived and the
	// front end holds it in memory only.
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["access_token"] != "an-access-token" {
		t.Errorf("access_token = %v, want it preserved", decoded["access_token"])
	}
}
