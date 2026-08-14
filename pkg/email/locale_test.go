// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import "testing"

// TestReplaceVar covers the tiny substitution the templates rely on, including the
// escaping that keeps a participant's name from becoming markup. It moved here with the
// function: it used to be the only test any of the five copies had.
func TestReplaceVar(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		vari  string
		value string
		want  string
	}{
		{name: "substitutes", in: "Hi {{.ParticipantName}}", vari: "ParticipantName", value: "Ada", want: "Hi Ada"},
		{name: "leaves other placeholders alone", in: "Hi {{.Other}}", vari: "ParticipantName", value: "Ada", want: "Hi {{.Other}}"},
		{name: "substitutes every occurrence", in: "{{.X}}/{{.X}}", vari: "X", value: "a", want: "a/a"},
		{
			// Deliberately not escaped here. html/template escapes the value when the
			// body is rendered; doing it twice would show the reader the entities.
			// internal/auth/handlers/email_body_test.go pins the end result.
			name:  "passes markup through for the renderer to escape",
			in:    "Hi {{.ParticipantName}}",
			vari:  "ParticipantName",
			value: "<b>",
			want:  "Hi <b>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceVar(tt.in, tt.vari, tt.value); got != tt.want {
				t.Errorf("ReplaceVar() = %q, want %q", got, tt.want)
			}
		})
	}
}
