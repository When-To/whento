// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// The two files this serves in place of what http-swagger would.
//
// Kept beside the code that serves them, the way the mail templates sit beside the handlers
// that render them.
var (
	//go:embed swaggerui/index.html
	swaggerIndex []byte

	//go:embed swaggerui/swagger-initializer.js
	swaggerInitializer []byte
)

// swaggerHandler serves the API documentation with no inline script.
//
// http-swagger renders an index whose only means of building the interface is an inline
// <script> calling SwaggerUIBundle. script-src is 'self', so the browser refused it: the
// bundle loaded, #swagger-ui stayed empty, and /swagger was a blank page in production.
//
// Nothing in that library could fix it from the outside. The index is an unexported const
// parsed inside the handler; its Config carries no nonce and no hash option; and
// BeforeScript, AfterScript, Plugins and UIConfig all inject into that same inline block,
// so they make the problem worse. A 'sha256-' allowance was the other candidate and was
// dropped: the block is a template rendered from config, so the digest breaks on any option
// change or library bump — a CSP that silently stops matching is worse than one that never
// did.
//
// So the index is ours, modelled on the CSP-clean dist/index.html that swaggo/files already
// ships and that only the interception above makes unreachable. The initialiser is ours for
// a duller reason: theirs points at petstore.swagger.io.
//
// Everything else — doc.json, the bundles, the stylesheets, the favicons — still comes from
// the upstream handler untouched.
func swaggerHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The wildcard is the same trailing segment http-swagger switches on, without
		// re-deriving it from RequestURI. An empty one falls through, so the library keeps
		// issuing its own redirect to index.html — which lands back here.
		switch chi.URLParam(r, "*") {
		case "index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(swaggerIndex)

		case "swagger-initializer.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write(swaggerInitializer)

		default:
			httpSwagger.WrapHandler(w, r)
		}
	}
}
