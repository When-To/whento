// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/whento/pkg/middleware"
)

// WithAuth adds authentication context to a request for testing authenticated endpoints
func WithAuth(req *http.Request, userID, role string) *http.Request {
	ctx := req.Context()
	ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	ctx = context.WithValue(ctx, middleware.UserRoleKey, role)
	return req.WithContext(ctx)
}

// WithURLParams adds Chi URL parameters to a request for testing routes with path parameters
func WithURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// MakeJSONRequest creates an HTTP request with JSON body for testing.
//
// An unencodable body would produce an empty request rather than an obvious failure,
// so the error is folded into the body instead of being dropped: the handler under
// test then rejects it and the test fails where it is readable.
func MakeJSONRequest(method, url string, body interface{}) *http.Request {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		buf.Reset()
		fmt.Fprintf(&buf, "testutil: could not encode request body: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, url, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// MakeRequest creates a basic HTTP request for testing
func MakeRequest(method, url string) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, url, nil)
}
