// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import "strings"

// EmailMatches checks if an email matches any of the allowed patterns.
// Patterns support wildcards:
//   - "*" matches all emails
//   - "*@domain.com" matches all emails from domain.com
//   - "user@*" matches user from any domain
//   - "exact@email.com" matches exact email
func EmailMatches(email string, allowedPatterns []string) bool {
	email = strings.ToLower(strings.TrimSpace(email))

	// Empty email never matches
	if email == "" {
		return false
	}

	for _, pattern := range allowedPatterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))

		// Empty pattern, skip
		if pattern == "" {
			continue
		}

		// Pattern "*" matches everything
		if pattern == "*" {
			return true
		}

		// Exact match
		if pattern == email {
			return true
		}

		// Pattern with wildcard
		if strings.Contains(pattern, "*") {
			if matchWildcardEmail(email, pattern) {
				return true
			}
		}
	}

	return false
}

// matchWildcardEmail matches email against a pattern with wildcard
func matchWildcardEmail(email, pattern string) bool {
	emailLocal, emailDomain, okEmail := strings.Cut(email, "@")
	patternLocal, patternDomain, okPattern := strings.Cut(pattern, "@")
	// Exactly one @ on each side.
	if !okEmail || !okPattern || strings.Contains(emailDomain, "@") || strings.Contains(patternDomain, "@") {
		return false
	}
	return (patternLocal == "*" || patternLocal == emailLocal) &&
		(patternDomain == "*" || patternDomain == emailDomain)
}
