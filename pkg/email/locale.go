// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package email

import "strings"

// ReplaceVar substitutes {{.VarName}} in a locale string with a value.
//
// The locale JSON files carry their own placeholders — "Hello {{.DisplayName}}," — which
// are not executed as templates: each mail builder substitutes them by hand before
// handing the result to the template that lays the message out. Five byte-identical
// copies of this function existed, three of them differing only by a suffix invented to
// dodge a same-package name collision.
//
// It does not escape. It used to, because the layout templates were text/template and
// nothing else would have. They are html/template now, so the value is escaped when the
// body is rendered — escaping here as well would show the reader "&lt;b&gt;" where the
// display name said "<b>". Callers pass the result as a plain string so the engine
// escapes it; only locale strings holding deliberate markup are typed template.HTML.
func ReplaceVar(str, varName, value string) string {
	placeholder := "{{." + varName + "}}"

	return strings.ReplaceAll(str, placeholder, value)
}
