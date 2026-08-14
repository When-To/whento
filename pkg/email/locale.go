// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import (
	"html"
	"strings"
)

// ReplaceVar substitutes {{.VarName}} in a locale string with an HTML-escaped value.
//
// The locale JSON files carry their own placeholders — "Hello {{.DisplayName}}," — which
// are not executed as templates: each mail builder substitutes them by hand before
// handing the result to the template that lays the message out. Five byte-identical
// copies of this function existed, three of them differing only by a suffix invented to
// dodge a same-package name collision.
//
// The escaping is what keeps a display name from carrying markup into a mail that
// someone else reads.
func ReplaceVar(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"

	return strings.ReplaceAll(str, placeholder, html.EscapeString(value))
}
