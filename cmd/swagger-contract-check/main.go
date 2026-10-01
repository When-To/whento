// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

// swagger-contract-check asserts the documented password ceiling survives a
// swagger regeneration.
//
// The runtime validator enforces 72 *bytes* via the custom `maxbytes` rule; swag
// does not understand that tag, so the Swagger `maxLength: 72` comes only from
// the paired `max=72` annotation on the same field. A re-annotator that drops
// the `max` tag would silently remove the documented ceiling, and the generated
// TypeScript cannot catch it — lengths are not encoded in the types.
//
// Reading swagger.json with the standard library keeps this portable: it is a
// small dependency-light replacement for the yq one-liner it used to be, which
// declared an external tool the CI image never installed.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// limits mirrors SWAGGER_PASSWORD_LIMITS in the Makefile: "model:property".
var limits = [][2]string{
	{"models.BootstrapRequest", "password"},
	{"models.ChangePasswordRequest", "new_password"},
	{"models.RegisterRequest", "password"},
	{"models.ResetPasswordRequest", "new_password"},
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	path := filepath.Join(root, "docs", "swagger", "swagger.json")
	if err := check(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("OK: all four password fields keep maxLength: 72")
}

func check(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	var doc struct {
		Definitions map[string]struct {
			Properties map[string]struct {
				MaxLength *int `json:"maxLength"`
			} `json:"properties"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	for _, limit := range limits {
		model, prop := limit[0], limit[1]

		definition, ok := doc.Definitions[model]
		if !ok {
			return fmt.Errorf("model %s is missing from the Swagger definitions; keep the swag annotations on it", model)
		}
		property, ok := definition.Properties[prop]
		if !ok {
			return fmt.Errorf("model %s.%s is missing from the Swagger definitions; keep the swag annotations on it", model, prop)
		}
		if property.MaxLength == nil || *property.MaxLength != 72 {
			got := "<nil>"
			if property.MaxLength != nil {
				got = fmt.Sprint(*property.MaxLength)
			}
			return fmt.Errorf("%s.%s lost maxLength: 72 (got: %s); keep the paired max=72 annotation alongside maxbytes=72, then re-run 'make swagger-generate'", model, prop, got)
		}
	}

	return nil
}
