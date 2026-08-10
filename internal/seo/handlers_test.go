// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package seo

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// What these files say is a privacy decision, not a formatting one. A self-hosted
// instance must not invite crawlers, and no build may advertise a participant calendar
// — the URL is the secret, so a sitemap entry would hand it to a search engine.

func serve(t *testing.T, h *Handler, handler func(http.ResponseWriter, *http.Request)) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	return rec
}

func TestRobotsDisallowsEverythingWhenNotCloud(t *testing.T) {
	tests := []struct {
		name          string
		buildType     string
		disableRobots bool
	}{
		{name: "self-hosted", buildType: "selfhosted"},
		{name: "self-hosted with robots explicitly disabled", buildType: "selfhosted", disableRobots: true},
		{name: "cloud with robots explicitly disabled", buildType: "cloud", disableRobots: true},
		{name: "an unrecognised build type", buildType: "something-else"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler("https://example.test", tt.disableRobots, tt.buildType)
			body := serve(t, h, h.HandleRobotsTxt).Body.String()

			if !strings.Contains(body, "Disallow: /\n") {
				t.Errorf("robots.txt does not disallow everything:\n%s", body)
			}
			// A blanket Allow would undo the Disallow above it.
			if strings.Contains(body, "Allow: /") {
				t.Errorf("robots.txt still allows crawling:\n%s", body)
			}
			if strings.Contains(body, "Sitemap:") {
				t.Errorf("a disallowed instance still advertises a sitemap:\n%s", body)
			}
		})
	}
}

func TestRobotsAllowsPublicPagesOnCloud(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	body := serve(t, h, h.HandleRobotsTxt).Body.String()

	for _, allowed := range []string{"/why-whento", "/privacy", "/terms", "/login", "/register"} {
		if !strings.Contains(body, "Allow: "+allowed) {
			t.Errorf("robots.txt does not allow %s:\n%s", allowed, body)
		}
	}

	if !strings.Contains(body, "Sitemap: https://whento.example/sitemap.xml") {
		t.Errorf("robots.txt does not point at the sitemap:\n%s", body)
	}
}

// TestRobotsBlocksPrivateRoutes is the one that matters. A participant calendar URL is
// its own authorisation, so /c/ must never be crawlable.
func TestRobotsBlocksPrivateRoutes(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	body := serve(t, h, h.HandleRobotsTxt).Body.String()

	for _, blocked := range []string{"/c/", "/dashboard", "/calendars/", "/settings", "/admin/", "/api/"} {
		if !strings.Contains(body, "Disallow: "+blocked) {
			t.Errorf("robots.txt does not block %s:\n%s", blocked, body)
		}
	}
}

// TestRobotsAdvertisesNoRemovedRoute guards against the drift this test was written
// after finding: robots.txt still allowed /pricing and blocked /billing, /cart and
// /checkout long after the billing pages were deleted, so crawlers were pointed at 404s.
func TestRobotsAdvertisesNoRemovedRoute(t *testing.T) {
	for _, buildType := range []string{"cloud", "selfhosted"} {
		h := NewHandler("https://whento.example", false, buildType)
		body := serve(t, h, h.HandleRobotsTxt).Body.String()

		for _, gone := range []string{"/pricing", "/billing", "/cart", "/checkout"} {
			if strings.Contains(body, gone) {
				t.Errorf("%s build still mentions the removed route %s:\n%s", buildType, gone, body)
			}
		}
	}
}

func TestRobotsContentType(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	rec := serve(t, h, h.HandleRobotsTxt)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
}

func TestSitemapIsEmptyWhenNotCloud(t *testing.T) {
	for _, tt := range []struct {
		name          string
		buildType     string
		disableRobots bool
	}{
		{name: "self-hosted", buildType: "selfhosted"},
		{name: "cloud with robots disabled", buildType: "cloud", disableRobots: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler("https://whento.example", tt.disableRobots, tt.buildType)
			body := serve(t, h, h.HandleSitemapXML).Body.String()

			if strings.Contains(body, "<loc>") {
				t.Errorf("a private instance published URLs:\n%s", body)
			}
			assertWellFormedXML(t, body)
		})
	}
}

func TestSitemapListsThePublicPagesOnCloud(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	body := serve(t, h, h.HandleSitemapXML).Body.String()

	for _, page := range []string{"/", "/why-whento", "/privacy", "/terms"} {
		if !strings.Contains(body, "<loc>https://whento.example"+page+"</loc>") {
			t.Errorf("the sitemap omits %s:\n%s", page, body)
		}
	}

	assertWellFormedXML(t, body)
}

func TestSitemapNeverPublishesACalendar(t *testing.T) {
	// The same privacy rule as robots.txt, from the other direction: a calendar URL in
	// a sitemap is a calendar URL handed to a search engine.
	h := NewHandler("https://whento.example", false, "cloud")
	body := serve(t, h, h.HandleSitemapXML).Body.String()

	for _, private := range []string{"/c/", "/dashboard", "/settings", "/admin"} {
		if strings.Contains(body, "<loc>https://whento.example"+private) {
			t.Errorf("the sitemap publishes %s:\n%s", private, body)
		}
	}
}

func TestSitemapMentionsNoRemovedRoute(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	body := serve(t, h, h.HandleSitemapXML).Body.String()

	if strings.Contains(body, "/pricing") {
		t.Errorf("the sitemap still points crawlers at /pricing, which no longer exists:\n%s", body)
	}
}

func TestSitemapContentType(t *testing.T) {
	h := NewHandler("https://whento.example", false, "cloud")
	rec := serve(t, h, h.HandleSitemapXML)

	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/xml") {
		t.Errorf("Content-Type = %q, want application/xml", got)
	}
}

func TestSitemapUsesTheConfiguredURL(t *testing.T) {
	// A wrong base URL makes every entry point at the wrong host, which is worse than
	// publishing nothing.
	h := NewHandler("https://calendar.example.org", false, "cloud")
	body := serve(t, h, h.HandleSitemapXML).Body.String()

	if !strings.Contains(body, "<loc>https://calendar.example.org/</loc>") {
		t.Errorf("the sitemap does not use the configured app URL:\n%s", body)
	}
	if strings.Contains(body, "whento.example") {
		t.Errorf("the sitemap contains a hard-coded host:\n%s", body)
	}
}

// assertWellFormedXML catches an unbalanced tag, which a string check never would.
func assertWellFormedXML(t *testing.T, body string) {
	t.Helper()

	decoder := xml.NewDecoder(strings.NewReader(body))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("the sitemap is not well-formed XML: %v\n%s", err, body)
		}
	}
}
