// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/httputil"
	"github.com/whento/pkg/middleware"
)

// newRouter builds the application router: global middleware, then one
// registration call per business domain, then the catch-alls.
//
// The order of the calls below is the order chi will see them, and two parts of
// it are load-bearing rather than cosmetic — see registerFallbacks.
//
// spa is the frontend handler. It is a parameter rather than something built
// here so that the router can be assembled — and walked — without the embedded
// frontend, which is what the route table test does.
func newRouter(d *deps, h *handlers, spa http.Handler) chi.Router {
	r := chi.NewRouter()

	// Global middleware.
	//
	// chi's RealIP is deliberately absent, and must stay absent: it overwrites
	// r.RemoteAddr from X-Forwarded-For / X-Real-IP for *every* request, before
	// anything gets to ask whether the connection came from a trusted proxy. That
	// turns TRUSTED_PROXIES (below) into decoration and makes every per-IP rate
	// limit a client-chosen header away from being bypassed. Proxy headers are
	// honoured in exactly one place instead — middleware.IPKeyFunc — and only for
	// connections that actually originate from a configured proxy.
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	// Outside Recoverer, so a panic is counted as the 500 the client received
	// rather than not counted at all.
	r.Use(middleware.Metrics)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.LimitRequestSize(1 * 1024 * 1024)) // 1MB max payload
	r.Use(middleware.CORS(d.cfg.CORSOrigins))

	// Configure trusted proxies for X-Forwarded-For validation
	middleware.SetTrustedProxies(d.cfg.TrustedProxies)

	// Health routes (use auth health handler as primary)
	r.Get("/api/health", h.health.Health)
	r.Get("/api/ready", h.health.Ready)

	registerAuthRoutes(r, d, h)
	registerPasskeyRoutes(r, d, h)
	registerMFARoutes(r, d, h)
	registerCalendarRoutes(r, d, h)
	registerAvailabilityRoutes(r, d, h)
	RegisterQuotaRoutes(r, d.quota, d.jwtManager, d.cacheStore)
	registerICSRoutes(r, d, h)
	registerSEORoutes(r, h)

	// ========== SWAGGER DOCUMENTATION ==========
	// swaggerHandler, not httpSwagger.WrapHandler: the library's index builds the UI from
	// an inline script the CSP refuses. See cmd/swagger.go.
	r.Get("/swagger/*", swaggerHandler())

	registerFallbacks(r, spa)

	return r
}

// registerFallbacks mounts what answers a request no route above claimed. It
// has to run last: r.NotFound only reaches the sub-routers that are already
// mounted.
func registerFallbacks(r chi.Router, spa http.Handler) {
	// An unmatched /api path is a mistake, not a deep link. Serving the SPA for it —
	// 200, and a page of HTML — makes a typo in a route look like success to any
	// client that checks the status and not the content type. It hid a broken seed
	// script, and it hid a health check in this repository's own CI pointing at
	// /api/v1/health, a route that has never existed.
	apiNotFound := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httputil.Error(w, http.StatusNotFound, httputil.ErrCodeNotFound, "Endpoint not found")
	})

	// Two registrations are needed, because a miss arrives by one of two paths.
	// A path with no matching sub-router prefix (/api/v1/nope) falls through to the
	// "/*" catch-all below, so it has to be intercepted here first.
	r.Handle("/api/*", apiNotFound)

	// A path that *does* match a sub-router prefix but no route inside it
	// (/api/v1/calendars/xyz/summary) is answered by that sub-router's NotFound, which
	// otherwise defaults to chi's plain-text one. Setting it here, after every route is
	// registered, propagates it to the sub-routers that do not already have one — so
	// every /api miss now answers with the same JSON envelope. Non-API paths never
	// reach it: "/*" matches them first.
	r.NotFound(apiNotFound)

	// Serve frontend on all non-API routes (SPA fallback)
	r.Handle("/*", spa)
}

// registerSEORoutes mounts robots.txt and sitemap.xml, whose content differs
// between the cloud and self-hosted variants.
func registerSEORoutes(r chi.Router, h *handlers) {
	r.Get("/robots.txt", h.seo.HandleRobotsTxt)
	r.Get("/sitemap.xml", h.seo.HandleSitemapXML)
}
