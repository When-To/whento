// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package seo

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	appURL        string
	disableRobots bool
	buildType     string // "cloud" or "selfhosted"
}

func NewHandler(appURL string, disableRobots bool, buildType string) *Handler {
	return &Handler{
		appURL:        appURL,
		disableRobots: disableRobots,
		buildType:     buildType,
	}
}

// HandleRobotsTxt serves a dynamic robots.txt file
//
//	@Summary		Get robots.txt
//	@Description	Returns a dynamically generated robots.txt file. For Cloud builds, allows public pages and blocks private routes. For Self-hosted builds, disallows all robots by default for privacy.
//	@Tags			SEO
//	@Produce		plain
//	@Success		200	{string}	string	"robots.txt content"
//	@Router			/robots.txt [get]
func (h *Handler) HandleRobotsTxt(w http.ResponseWriter, r *http.Request) {
	content := "# WhenTo - Robots.txt\n# Generated dynamically based on deployment configuration\n\n"

	// SEO only enabled for cloud builds, unless explicitly disabled
	if h.disableRobots || h.buildType != "cloud" {
		// Disallow all robots for self-hosted or when explicitly disabled
		content += "User-agent: *\nDisallow: /\n"
		if h.buildType == "selfhosted" {
			content += "\n# Self-hosted instance - SEO disabled for privacy\n"
		}
	} else {
		content += cloudRobots + h.appURL + "/sitemap.xml\n"
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content))
}

// cloudRobots allows the public cloud pages and blocks the private routes; the
// sitemap URL completes its last line.
const cloudRobots = `User-agent: *
Allow: /
Allow: /why-whento
Allow: /privacy
Allow: /terms
Allow: /login
Allow: /register

# Block private and user-specific routes
Disallow: /c/
Disallow: /dashboard
Disallow: /calendars/
Disallow: /settings
Disallow: /admin/
Disallow: /api/

# Sitemap
Sitemap: `

// HandleSitemapXML serves a dynamic sitemap.xml file
//
//	@Summary		Get sitemap.xml
//	@Description	Returns a dynamically generated XML sitemap. For Cloud builds, includes public marketing pages (home, why-whento, legal). For Self-hosted builds, returns an empty sitemap for privacy.
//	@Tags			SEO
//	@Produce		xml
//	@Success		200	{string}	string	"XML sitemap content"
//	@Router			/sitemap.xml [get]
func (h *Handler) HandleSitemapXML(w http.ResponseWriter, r *http.Request) {
	// SEO only enabled for cloud builds
	if h.disableRobots || h.buildType != "cloud" {
		// Return minimal response if robots disabled or self-hosted
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <!-- Sitemap disabled for this instance -->
</urlset>`))
		return
	}

	var content strings.Builder
	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
        xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
        xsi:schemaLocation="http://www.sitemaps.org/schemas/sitemap/0.9
        http://www.sitemaps.org/schemas/sitemap/0.9/sitemap.xsd">

`)

	today := time.Now().Format("2006-01-02")
	for _, page := range sitemapPages {
		if page.comment != "" {
			fmt.Fprintf(&content, "  <!-- %s -->\n", page.comment)
		}
		fmt.Fprintf(&content, "  <url>\n    <loc>%s%s</loc>\n    <changefreq>%s</changefreq>\n    <priority>%s</priority>\n    <lastmod>%s</lastmod>\n  </url>\n\n",
			h.appURL, page.path, page.changefreq, page.priority, today)
	}

	content.WriteString("  <!-- Note: User calendars (/c/*) are intentionally excluded for privacy -->\n\n")
	content.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content.String()))
}

// sitemapPages are the public pages of a cloud sitemap; a comment, when set,
// is written just before its entry.
var sitemapPages = []struct{ comment, path, changefreq, priority string }{
	{"Public marketing pages", "/", "monthly", "1.0"},
	{"", "/why-whento", "monthly", "0.8"},
	{"Legal pages", "/privacy", "yearly", "0.3"},
	{"", "/terms", "yearly", "0.3"},
}
