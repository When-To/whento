// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package seo

import (
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
	var content strings.Builder

	content.WriteString("# WhenTo - Robots.txt\n")
	content.WriteString("# Generated dynamically based on deployment configuration\n\n")

	// SEO only enabled for cloud builds, unless explicitly disabled
	if h.disableRobots || h.buildType != "cloud" {
		// Disallow all robots for self-hosted or when explicitly disabled
		content.WriteString("User-agent: *\n")
		content.WriteString("Disallow: /\n")
		if h.buildType == "selfhosted" {
			content.WriteString("\n# Self-hosted instance - SEO disabled for privacy\n")
		}
	} else {
		content.WriteString("User-agent: *\n")
		content.WriteString("Allow: /\n")

		// Cloud-specific public routes
		if h.buildType == "cloud" {
			content.WriteString("Allow: /pricing\n")
			content.WriteString("Allow: /why-whento\n")
			content.WriteString("Allow: /privacy\n")
			content.WriteString("Allow: /terms\n")
			content.WriteString("Allow: /login\n")
			content.WriteString("Allow: /register\n")
			content.WriteString("\n")
		}

		// Block private/authenticated routes
		content.WriteString("# Block private and user-specific routes\n")
		content.WriteString("Disallow: /c/\n")
		content.WriteString("Disallow: /dashboard\n")
		content.WriteString("Disallow: /calendars/\n")
		content.WriteString("Disallow: /settings\n")
		content.WriteString("Disallow: /admin/\n")
		content.WriteString("Disallow: /api/\n")

		if h.buildType == "cloud" {
			content.WriteString("Disallow: /billing\n")
			content.WriteString("Disallow: /cart\n")
			content.WriteString("Disallow: /checkout\n")
		}

		content.WriteString("\n# Sitemap\n")
		content.WriteString("Sitemap: " + h.appURL + "/sitemap.xml\n")
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content.String()))
}

// HandleSitemapXML serves a dynamic sitemap.xml file
//
//	@Summary		Get sitemap.xml
//	@Description	Returns a dynamically generated XML sitemap. For Cloud builds, includes public marketing pages (home, pricing, why-whento, legal). For Self-hosted builds, returns an empty sitemap for privacy.
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

	content.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	content.WriteString("\n")
	content.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"`)
	content.WriteString("\n")
	content.WriteString(`        xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`)
	content.WriteString("\n")
	content.WriteString(`        xsi:schemaLocation="http://www.sitemaps.org/schemas/sitemap/0.9`)
	content.WriteString("\n")
	content.WriteString(`        http://www.sitemaps.org/schemas/sitemap/0.9/sitemap.xsd">`)
	content.WriteString("\n\n")

	today := time.Now().Format("2006-01-02")

	// Always include home page
	content.WriteString("  <!-- Public marketing pages -->\n")
	content.WriteString("  <url>\n")
	content.WriteString("    <loc>" + h.appURL + "/</loc>\n")
	content.WriteString("    <changefreq>monthly</changefreq>\n")
	content.WriteString("    <priority>1.0</priority>\n")
	content.WriteString("    <lastmod>" + today + "</lastmod>\n")
	content.WriteString("  </url>\n\n")

	// Cloud-specific pages
	if h.buildType == "cloud" {
		content.WriteString("  <url>\n")
		content.WriteString("    <loc>" + h.appURL + "/pricing</loc>\n")
		content.WriteString("    <changefreq>monthly</changefreq>\n")
		content.WriteString("    <priority>0.9</priority>\n")
		content.WriteString("    <lastmod>" + today + "</lastmod>\n")
		content.WriteString("  </url>\n\n")

		content.WriteString("  <url>\n")
		content.WriteString("    <loc>" + h.appURL + "/why-whento</loc>\n")
		content.WriteString("    <changefreq>monthly</changefreq>\n")
		content.WriteString("    <priority>0.8</priority>\n")
		content.WriteString("    <lastmod>" + today + "</lastmod>\n")
		content.WriteString("  </url>\n\n")

		content.WriteString("  <!-- Legal pages -->\n")
		content.WriteString("  <url>\n")
		content.WriteString("    <loc>" + h.appURL + "/privacy</loc>\n")
		content.WriteString("    <changefreq>yearly</changefreq>\n")
		content.WriteString("    <priority>0.3</priority>\n")
		content.WriteString("    <lastmod>" + today + "</lastmod>\n")
		content.WriteString("  </url>\n\n")

		content.WriteString("  <url>\n")
		content.WriteString("    <loc>" + h.appURL + "/terms</loc>\n")
		content.WriteString("    <changefreq>yearly</changefreq>\n")
		content.WriteString("    <priority>0.3</priority>\n")
		content.WriteString("    <lastmod>" + today + "</lastmod>\n")
		content.WriteString("  </url>\n\n")
	}

	content.WriteString("  <!-- Note: User calendars (/c/*) are intentionally excluded for privacy -->\n\n")
	content.WriteString("</urlset>\n")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(content.String()))
}
