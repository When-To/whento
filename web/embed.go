// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed dist/*
//go:embed dist/assets/*
var frontendFS embed.FS

// PageMeta holds SEO meta tags for a specific page
type PageMeta struct {
	Title         string
	Description   string
	OGTitle       string
	OGDescription string
	OGImage       string
	Canonical     string
	NoIndex       bool   // Prevent search engine indexing
	JSONLD        string // JSON-LD structured data for rich snippets
}

// SPAHandler serves the embedded frontend with SPA fallback
type SPAHandler struct {
	staticFS   http.Handler
	indexHTML  []byte
	fileSystem fs.FS
	appURL     string
	buildType  string
}

// NewSPAHandler creates a new SPA handler from the embedded frontend
func NewSPAHandler(appURL string, buildType string) (*SPAHandler, error) {
	// Get the dist subdirectory
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return nil, err
	}

	// Read index.html for SPA fallback
	indexHTML, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		return nil, err
	}

	return &SPAHandler{
		staticFS:   http.FileServer(http.FS(distFS)),
		indexHTML:  indexHTML,
		fileSystem: distFS,
		appURL:     appURL,
		buildType:  buildType,
	}, nil
}

// getJSONLDForHome returns JSON-LD structured data for the home page
func (h *SPAHandler) getJSONLDForHome() string {
	return fmt.Sprintf(`{
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebApplication",
      "name": "WhenTo",
      "url": "%s",
      "description": "Free recurring date poll and collaborative calendar. Best Doodle alternative for scheduling regular events with automatic iCal sync.",
      "applicationCategory": "SchedulingApplication",
      "operatingSystem": "Web Browser",
      "offers": {
        "@type": "Offer",
        "price": "0",
        "priceCurrency": "EUR"
      },
      "featureList": [
        "Recurring date polls",
        "Collaborative calendars",
        "iCal synchronization",
        "Google Calendar integration",
        "Outlook integration",
        "Apple Calendar integration",
        "Recurring availability patterns",
        "Self-hosting option"
      ]
    },
    {
      "@type": "Organization",
      "name": "WhenTo",
      "url": "%s",
      "logo": "%s/logo.png",
      "sameAs": [
        "https://github.com/When-To/whento"
      ]
    }
  ]
}`, h.appURL, h.appURL, h.appURL)
}

// getJSONLDForWhyPage returns JSON-LD FAQ structured data for the "Why WhenTo" page
func (h *SPAHandler) getJSONLDForWhyPage() string {
	return `{
  "@context": "https://schema.org",
  "@type": "FAQPage",
  "mainEntity": [
    {
      "@type": "Question",
      "name": "What is WhenTo?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "WhenTo is a free collaborative calendar and recurring date poll tool. Unlike Doodle or Framadate, WhenTo offers permanent calendars with recurring availability patterns and automatic iCal synchronization."
      }
    },
    {
      "@type": "Question",
      "name": "How is WhenTo different from Doodle?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "WhenTo is designed for recurring events. Instead of creating a new poll every week, you create one calendar that persists. Participants can set recurring availability (e.g., 'every Tuesday evening'), and validated events sync automatically to Google Calendar, Outlook, or Apple Calendar."
      }
    },
    {
      "@type": "Question",
      "name": "Is WhenTo free?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "Yes, WhenTo offers a free plan with 3 calendars. Paid plans are available for teams needing more calendars: Pro (30 calendars) for 25€/year + VAT and Power (unlimited) for 100€/year + VAT."
      }
    },
    {
      "@type": "Question",
      "name": "Can I self-host WhenTo?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "Yes, WhenTo is available under the Business Source License (BSL). You can self-host it for personal or internal use. The source code is available on GitHub."
      }
    },
    {
      "@type": "Question",
      "name": "What are the best use cases for WhenTo?",
      "acceptedAnswer": {
        "@type": "Answer",
        "text": "WhenTo is perfect for groups that meet regularly: RPG and board game sessions, sports teams, music bands, and recurring team meetings. Any group that needs to find common availability on a recurring basis."
      }
    }
  ]
}`
}

// getMetaForRoute returns SEO meta tags for a specific route
func (h *SPAHandler) getMetaForRoute(path string) PageMeta {
	// Default meta tags (home page) - optimized for SEO with bilingual keywords
	defaultMeta := PageMeta{
		Title:         "WhenTo - Recurring Date Poll & Collaborative Calendar | Free Doodle Alternative",
		Description:   "Free recurring date poll and collaborative calendar. Best Doodle & Framadate alternative for scheduling regular events: RPG sessions, sports, music. Automatic iCal sync with Google Calendar, Outlook. Sondage de date récurrent gratuit.",
		OGTitle:       "WhenTo - Recurring Date Poll & Collaborative Calendar",
		OGDescription: "Free Doodle alternative for recurring events. Create collaborative calendars, track availability, sync automatically with Google Calendar & Outlook. Perfect for RPG, sports, music groups.",
		OGImage:       h.appURL + "/og-home.png",
		Canonical:     h.appURL + path,
	}

	// Only provide SEO for cloud builds
	if h.buildType != "cloud" {
		return defaultMeta
	}

	// Route-specific meta tags
	switch path {
	case "/", "/home":
		defaultMeta.JSONLD = h.getJSONLDForHome()
		return defaultMeta

	case "/pricing":
		return PageMeta{
			Title:         "Pricing - WhenTo | Free Date Poll & Paid Plans",
			Description:   "WhenTo pricing: Free plan with 3 calendars, Pro with 30 calendars (25€/year + VAT), Power unlimited (100€/year + VAT). Best value Doodle alternative for recurring events.",
			OGTitle:       "WhenTo Pricing - Free & Paid Plans for Teams",
			OGDescription: "Start free with 3 calendars. Upgrade to Pro (30 calendars) or Power (unlimited) as your team grows. Annual billing, no hidden fees.",
			OGImage:       h.appURL + "/og-pricing.png",
			Canonical:     h.appURL + "/pricing",
		}

	case "/why-whento":
		return PageMeta{
			Title:         "Why WhenTo? Best Doodle Alternative for Recurring Events | Date Poll Comparison",
			Description:   "Why WhenTo beats Doodle & Framadate for recurring events: permanent calendars, recurring availability, automatic iCal sync. Compare features and discover the smart way to schedule.",
			OGTitle:       "Why Choose WhenTo Over Doodle?",
			OGDescription: "Tired of creating new Doodle polls every week? WhenTo offers permanent calendars, recurring availability, and automatic calendar sync. The smart alternative.",
			OGImage:       h.appURL + "/og-why.png",
			Canonical:     h.appURL + "/why-whento",
			JSONLD:        h.getJSONLDForWhyPage(),
		}

	case "/privacy":
		return PageMeta{
			Title:         "Privacy Policy - WhenTo",
			Description:   "Learn how WhenTo protects your data and respects your privacy. We're committed to transparency and security.",
			OGTitle:       "WhenTo Privacy Policy",
			OGDescription: "Your privacy matters. Read our privacy policy to understand how we handle your data.",
			OGImage:       h.appURL + "/og-home.png",
			Canonical:     h.appURL + "/privacy",
			NoIndex:       true, // Legal pages should not be indexed
		}

	case "/terms":
		return PageMeta{
			Title:         "Terms of Service - WhenTo",
			Description:   "Terms and conditions for using WhenTo. Understanding your rights and responsibilities.",
			OGTitle:       "WhenTo Terms of Service",
			OGDescription: "Terms and conditions for using WhenTo's collaborative calendar platform.",
			OGImage:       h.appURL + "/og-home.png",
			Canonical:     h.appURL + "/terms",
			NoIndex:       true, // Legal pages should not be indexed
		}

	default:
		// For calendar routes (/c/*), provide social sharing meta but with noindex
		if strings.HasPrefix(path, "/c/") {
			return PageMeta{
				Title:         "Join this WhenTo Calendar",
				Description:   "You've been invited to participate in a collaborative calendar. Click to share your availability!",
				OGTitle:       "Join this WhenTo Calendar",
				OGDescription: "📅 Collaborative event planning made easy. Click to share your availability and help the group find the perfect time!",
				OGImage:       h.appURL + "/og-calendar.png",
				Canonical:     h.appURL + path,
				NoIndex:       true, // User calendars should not be indexed
			}
		}

		return defaultMeta
	}
}

// injectMetaTags replaces the default meta tags in index.html with route-specific ones
func (h *SPAHandler) injectMetaTags(html []byte, meta PageMeta) []byte {
	htmlStr := string(html)

	// Replace title (match the existing title tag)
	if meta.Title != "" {
		// Find and replace the existing title tag
		titleStart := strings.Index(htmlStr, "<title>")
		if titleStart != -1 {
			titleEnd := strings.Index(htmlStr[titleStart:], "</title>") + titleStart + 8 // +8 for </title>
			if titleEnd > titleStart {
				htmlStr = htmlStr[:titleStart] + "<title>" + meta.Title + "</title>" + htmlStr[titleEnd:]
			}
		}
	}

	// Replace existing meta description instead of adding a new one
	if meta.Description != "" {
		descStart := strings.Index(htmlStr, `<meta name="description"`)
		if descStart != -1 {
			// Find the end of the existing description tag
			descEnd := strings.Index(htmlStr[descStart:], ">") + descStart + 1
			if descEnd > descStart {
				newDesc := fmt.Sprintf(`<meta name="description" content="%s">`, meta.Description)
				htmlStr = htmlStr[:descStart] + newDesc + htmlStr[descEnd:]
			}
		}
	}

	// Find the position after <meta charset="UTF-8" /> to inject our meta tags
	charsetPos := strings.Index(htmlStr, `<meta charset="UTF-8"`)
	if charsetPos == -1 {
		return html // If charset meta not found, return original
	}

	// Find the end of the charset meta tag (looking for />)
	insertPos := strings.Index(htmlStr[charsetPos:], "/>") + charsetPos + 2

	// Build meta tags to inject (WITHOUT description, as it's already replaced above)
	var metaTags strings.Builder
	metaTags.WriteString("\n    ")

	// Open Graph tags
	if meta.OGTitle != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta property="og:title" content="%s">`, meta.OGTitle))
		metaTags.WriteString("\n    ")
	}
	if meta.OGDescription != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta property="og:description" content="%s">`, meta.OGDescription))
		metaTags.WriteString("\n    ")
	}
	if meta.OGImage != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta property="og:image" content="%s">`, meta.OGImage))
		metaTags.WriteString("\n    ")
	}
	metaTags.WriteString(`<meta property="og:type" content="website">`)
	metaTags.WriteString("\n    ")

	// Twitter Card tags
	metaTags.WriteString(`<meta name="twitter:card" content="summary_large_image">`)
	metaTags.WriteString("\n    ")
	if meta.OGTitle != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta name="twitter:title" content="%s">`, meta.OGTitle))
		metaTags.WriteString("\n    ")
	}
	if meta.OGDescription != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta name="twitter:description" content="%s">`, meta.OGDescription))
		metaTags.WriteString("\n    ")
	}
	if meta.OGImage != "" {
		metaTags.WriteString(fmt.Sprintf(`<meta name="twitter:image" content="%s">`, meta.OGImage))
		metaTags.WriteString("\n    ")
	}

	// Canonical URL
	if meta.Canonical != "" {
		metaTags.WriteString(fmt.Sprintf(`<link rel="canonical" href="%s">`, meta.Canonical))
		metaTags.WriteString("\n    ")
	}

	// Noindex tag (for legal pages and user calendars)
	if meta.NoIndex {
		metaTags.WriteString(`<meta name="robots" content="noindex, nofollow">`)
		metaTags.WriteString("\n    ")
	}

	// JSON-LD structured data for rich snippets
	if meta.JSONLD != "" {
		metaTags.WriteString(`<script type="application/ld+json">`)
		metaTags.WriteString(meta.JSONLD)
		metaTags.WriteString(`</script>`)
		metaTags.WriteString("\n    ")
	}

	// Inject the meta tags
	result := htmlStr[:insertPos] + metaTags.String() + htmlStr[insertPos:]
	return []byte(result)
}

// ServeHTTP implements http.Handler
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Remove leading slash for fs operations
	fsPath := strings.TrimPrefix(path, "/")
	if fsPath == "" {
		fsPath = "index.html"
	}

	// Check if file exists
	_, err := fs.Stat(h.fileSystem, fsPath)
	if err != nil || fsPath == "index.html" {
		// File doesn't exist OR it's index.html - serve with injected meta tags for SEO
		meta := h.getMetaForRoute(path)
		injectedHTML := h.injectMetaTags(h.indexHTML, meta)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(injectedHTML)
		return
	}

	// File exists (assets, etc.) - serve it directly
	h.staticFS.ServeHTTP(w, r)
}
