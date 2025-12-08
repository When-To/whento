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
	// Split email into local and domain parts
	emailParts := strings.Split(email, "@")
	if len(emailParts) != 2 {
		return false
	}
	emailLocal := emailParts[0]
	emailDomain := emailParts[1]

	// Split pattern
	patternParts := strings.Split(pattern, "@")
	if len(patternParts) != 2 {
		return false
	}
	patternLocal := patternParts[0]
	patternDomain := patternParts[1]

	// Check local part
	if patternLocal != "*" && patternLocal != emailLocal {
		return false
	}

	// Check domain part
	if patternDomain != "*" && patternDomain != emailDomain {
		return false
	}

	return true
}
