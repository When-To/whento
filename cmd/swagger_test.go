// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// /swagger was a blank page in production: the library's index builds the interface from an
// inline <script>, and script-src is 'self'. The tests below are what keeps it from becoming
// one again — a browser refusing a script is silent server-side, so nothing else would.

// scriptOpenTag finds every <script …> opening tag. A tag without a src attribute carries
// its code in the document, which is exactly what script-src 'self' refuses.
var scriptOpenTag = regexp.MustCompile(`(?is)<script(\s[^>]*)?>`)

// htmlComment is stripped before the match. The index explains in a comment what it exists
// to avoid, and naming the tag there is not the same as carrying one — a browser parses no
// script out of a comment either.
var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

// jsBlockComment is stripped for the same reason. Only /* … */ and not // …, which would
// eat the second half of any https:// in the code the test is actually looking at.
var jsBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func TestSwaggerIndexCarriesNoInlineScript(t *testing.T) {
	r := testRouter(t, false)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger/index.html = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	for _, tag := range scriptOpenTag.FindAllString(htmlComment.ReplaceAllString(body, ""), -1) {
		if !strings.Contains(tag, "src=") {
			t.Errorf("the index carries an inline script, which script-src 'self' refuses: %s", tag)
		}
	}
	// The three files it must pull instead, all same-origin.
	for _, want := range []string{
		`src="./swagger-ui-bundle.js"`,
		`src="./swagger-ui-standalone-preset.js"`,
		`src="./swagger-initializer.js"`,
		`id="swagger-ui"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the index is missing %q:\n%s", want, body)
		}
	}
}

func TestSwaggerInitializerPointsAtThisBinary(t *testing.T) {
	r := testRouter(t, false)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/swagger-initializer.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger/swagger-initializer.js = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	code := jsBlockComment.ReplaceAllString(body, "")
	if !strings.Contains(code, "./doc.json") {
		t.Errorf("the initialiser does not load this binary's spec:\n%s", body)
	}
	// The upstream file is reachable through the same handler and hardcodes the public demo
	// in its code, not merely in a comment — serving it would render somebody else's API.
	if strings.Contains(code, "petstore.swagger.io") {
		t.Errorf("the upstream demo initialiser is being served:\n%s", body)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Errorf("Content-Type = %q, want a JavaScript type", got)
	}
}

// TestSwaggerStillDelegatesTheSpec guards the half that is not ours: only two paths are
// intercepted, and everything else has to reach the library untouched.
func TestSwaggerStillDelegatesTheSpec(t *testing.T) {
	r := testRouter(t, false)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger/doc.json = %d, want 200", rec.Code)
	}

	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("the spec is not JSON: %v", err)
	}
	if _, ok := spec["paths"]; !ok {
		t.Error("the spec carries no paths, so it is not the API description")
	}
}
