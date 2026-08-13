// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package logger

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Most of the leaks closed in this pass were not in code anybody could reach
// from a unit test: they were in a notification loop that queries three
// repositories, or in a fire-and-forget goroutine. Driving each of them would
// mean inventing a fixture for code that was never built to have one, and the
// fixture would then be the thing under test.
//
// This reads the source instead. It is the only guard that covers a log line
// added tomorrow, in a package nobody thought to write a privacy test for — and
// the leaks it looks for are all spelled the same way: a field named after the
// thing it must not contain.

// forbiddenFields are log field names that, by their name alone, promise to
// carry something the owner said is never written: an address, a credential, a
// person's name, a device.
//
// The check is on the name, not the value, and that is deliberate. A field
// called "email" holding something harmless is still a field the next person
// will fill with an address.
var forbiddenFields = map[string]string{
	"email":          "an email address — use a user_id, or logger.Fingerprint",
	"user_email":     "an email address — use a user_id, or logger.Fingerprint",
	"to":             "a recipient address — use logger.Fingerprint",
	"recipient":      "a recipient address — use logger.Fingerprint",
	"recipients":     "recipient addresses — log a count, or logger.Fingerprint",
	"token":          "a credential; in WhenTo the calendar token *is* the authorisation",
	"public_token":   "the calendar credential",
	"ics_token":      "the ICS feed credential",
	"calendar_token": "the calendar credential",
	"access_token":   "a credential",
	"refresh_token":  "a credential",
	"magic_link":     "a credential",
	// Neither of these reads as a credential from its name, which is exactly why
	// both slipped past an earlier pass of this guard: a pub/sub topic in this
	// codebase *is* the calendar token, and a challenge id is the handle to a
	// live WebAuthn ceremony.
	"topic":            "the broadcast topic is the calendar token — use logger.Fingerprint",
	"challenge_id":     "the handle to a live passkey ceremony — use logger.Fingerprint",
	"participant_id":   "half of the participant capability — use logger.Fingerprint",
	"pid":              "half of the participant capability — use logger.Fingerprint",
	"recipient_id":     "a participant or user id — use logger.Fingerprint",
	"name":             "a person's name",
	"names":            "people's names — log a count",
	"display_name":     "a person's name",
	"participant_name": "a person's name",
	"existing_name":    "a person's name",
	"chat_id":          "a Telegram chat, which names a person — use logger.Fingerprint",
	"ip":               "an IP address",
	"remote_addr":      "an IP address",
	"client_ip":        "an IP address",
	"user_agent":       "a browser fingerprint",
	"password":         "a secret",
	"path":             "in WhenTo the request path carries the credential — log the route pattern",
	"url":              "a participant link carries the calendar token and the participant id",
	"query":            "a query string can carry a token",
}

// allowedExceptions are the field names deliberately kept, each because the log
// line is doing something the alternative cannot.
//
//   - reset_url: when SMTP is unconfigured the log *is* how a password reset is
//     delivered. It is written at warn, only on an instance with no mail set up,
//     and it is documented in docs/logging-and-privacy.md §5.
//   - webhook_url: truncated to its first twenty characters, which is the scheme
//     and host of a Discord or Slack endpoint and none of the secret path.
var allowedExceptions = map[string]bool{
	"reset_url":   true,
	"webhook_url": true,
}

// allowedAt lifts a field name in one file only, where the name is right but the
// value provably is not what the name usually means.
var allowedAt = map[string]string{
	// The Prometheus exposition path, a build-time constant on a listener of its
	// own. Not a request path and not reachable from one.
	"cmd/main.go:path": "metrics.MetricsPath",
}

// logMethods are the slog method names. A call whose first argument is a string
// literal and whose selector is one of these is a log line; nothing else in this
// codebase has that shape.
var logMethods = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true,
	"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
}

// attrConstructors are the typed slog helpers, whose first argument is the key.
var attrConstructors = map[string]bool{
	"String": true, "Int": true, "Int64": true, "Uint64": true,
	"Float64": true, "Bool": true, "Time": true, "Duration": true, "Any": true,
}

// scannedDirs are the trees this guard covers, relative to the repository root.
var scannedDirs = []string{"internal", "pkg", "cmd"}

func TestNoLogFieldIsNamedAfterPersonalData(t *testing.T) {
	root := repositoryRoot(t)

	for _, dir := range scannedDirs {
		t.Run(dir, func(t *testing.T) {
			target := filepath.Join(root, dir)
			if _, err := os.Stat(target); err != nil {
				// pkg/ is its own module and can be checked out on its own, in
				// which case there is nothing here to scan.
				t.Skipf("%s is not present", target)
			}

			findings := scanForForbiddenFields(t, root, target)
			for _, f := range findings {
				t.Errorf("%s: log field %q — %s", f.where, f.field, forbiddenFields[f.field])
			}
		})
	}
}

type finding struct {
	where string
	field string
}

func scanForForbiddenFields(t *testing.T, repoRoot, root string) []finding {
	t.Helper()

	var findings []finding
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Generated Swagger and the embedded frontend build are not ours.
			if d.Name() == "swagger" || d.Name() == "dist" || d.Name() == "node_modules" {
				return fs.SkipDir
			}

			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			switch {
			case logMethods[sel.Sel.Name] && len(call.Args) > 1 && isStringLit(call.Args[0]):
				// Key/value pairs follow the message, so the keys are the odd
				// positions.
				for i := 1; i < len(call.Args); i += 2 {
					if field, ok := stringLit(call.Args[i]); ok {
						if f, bad := check(field, rel, fset.Position(call.Args[i].Pos()).String()); bad {
							findings = append(findings, f)
						}
					}
				}
			case attrConstructors[sel.Sel.Name] && len(call.Args) > 0:
				if field, ok := stringLit(call.Args[0]); ok {
					if f, bad := check(field, rel, fset.Position(call.Args[0].Pos()).String()); bad {
						findings = append(findings, f)
					}
				}
			}

			return true
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	return findings
}

func check(field, file, where string) (finding, bool) {
	if allowedExceptions[field] {
		return finding{}, false
	}
	if _, lifted := allowedAt[file+":"+field]; lifted {
		return finding{}, false
	}
	if _, forbidden := forbiddenFields[field]; !forbidden {
		return finding{}, false
	}

	return finding{where: where, field: field}, true
}

func isStringLit(e ast.Expr) bool {
	_, ok := stringLit(e)

	return ok
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}

	return value, true
}

// repositoryRoot walks up from the test's working directory until it finds the
// tree that holds the scanned directories. Tests run in their own package
// directory, and pkg/ is a module of its own, so neither depth is fixed.
func repositoryRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "internal")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Skip("the repository root was not found; nothing to scan")

	return ""
}
