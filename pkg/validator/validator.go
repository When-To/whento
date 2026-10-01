// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package validator

import (
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New()

	// Use JSON tag names for error messages
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// Register custom validations
	_ = validate.RegisterValidation("locale", validateLocale)
	_ = validate.RegisterValidation("strongpassword", validateStrongPassword)
	_ = validate.RegisterValidation("maxbytes", validateMaxBytes)
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (v ValidationErrors) Error() string {
	var msgs []string
	for _, err := range v {
		msgs = append(msgs, err.Field+": "+err.Message)
	}
	return strings.Join(msgs, "; ")
}

// Validate validates a struct
func Validate(s interface{}) error {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return err
	}

	var errs ValidationErrors
	for _, e := range validationErrors {
		errs = append(errs, ValidationError{
			Field:   e.Field(),
			Message: getErrorMessage(e),
		})
	}

	return errs
}

// ValidateVar validates a single variable
func ValidateVar(field interface{}, tag string) error {
	return validate.Var(field, tag)
}

func getErrorMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return "this field is required"
	case "email":
		return "must be a valid email address"
	case "min":
		return "must be at least " + e.Param() + " characters"
	case "max":
		return "must be at most " + e.Param() + " characters"
	case "gte":
		return "must be greater than or equal to " + e.Param()
	case "lte":
		return "must be less than or equal to " + e.Param()
	case "maxbytes":
		return "must be at most " + e.Param() + " bytes"
	case "oneof":
		return "must be one of: " + e.Param()
	case "timezone":
		return "must be a valid IANA timezone"
	case "locale":
		return "must be one of: fr, en"
	case "uuid":
		return "must be a valid UUID"
	case "strongpassword":
		return "must be at least 12 characters with uppercase, lowercase, number, and special character"
	default:
		return "failed validation: " + e.Tag()
	}
}

func validateLocale(fl validator.FieldLevel) bool {
	locale := fl.Field().String()
	return locale == "fr" || locale == "en"
}

// validateMaxBytes enforces a ceiling on the *byte* length of a string, the unit
// bcrypt actually cares about (it rejects inputs over 72 bytes). go-playground's
// built-in max counts runes for strings, so a password made of multibyte
// characters could pass `max=72` and then blow up bcrypt with an internal error.
// This validator must run *before* hashing so the failure is a clean 400.
func validateMaxBytes(fl validator.FieldLevel) bool {
	limit, err := strconv.Atoi(fl.Param())
	if err != nil {
		return false
	}
	return len([]byte(fl.Field().String())) <= limit
}

// validateStrongPassword validates password complexity
// Requirements:
//   - Minimum 12 characters (UTF-8 code points — this is the unit users think
//     in; bcrypt's real ceiling is 72 bytes, enforced separately by maxbytes)
//   - At least 1 uppercase letter
//   - At least 1 lowercase letter
//   - At least 1 digit
//   - At least 1 special character
func validateStrongPassword(fl validator.FieldLevel) bool {
	password := fl.Field().String()

	// Minimum 12 characters. utf8.RuneCountInString, not len: len counts UTF-8
	// bytes, so a password of e.g. 10 accented characters (20 bytes) would pass
	// a byte check but not the user-visible "12 characters" rule — and the one
	// true authority a user sees (the frontend) counts code points via
	// Array.from. Counting runes here keeps the two halves in agreement.
	if utf8.RuneCountInString(password) < 12 {
		return false
	}

	// At least 1 uppercase letter
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(password)
	if !hasUpper {
		return false
	}

	// At least 1 lowercase letter
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(password)
	if !hasLower {
		return false
	}

	// At least 1 digit
	hasDigit := regexp.MustCompile(`[0-9]`).MatchString(password)
	if !hasDigit {
		return false
	}

	// At least 1 special character
	hasSpecial := regexp.MustCompile(`[!@#$%^&*(),.?":{}|<>_\-+=\[\]\\\/;'~` + "`" + `]`).MatchString(password)
	return hasSpecial
}
