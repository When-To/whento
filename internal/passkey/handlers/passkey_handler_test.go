// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/pkg/middleware"
	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/passkey/models"
	passkeyRepo "github.com/whento/whento/internal/passkey/repository"
	"github.com/whento/whento/internal/passkey/service"
)

// The registration and authentication ceremonies need an authenticator signing an
// attestation, which no test can produce, so those two endpoints stay uncovered on
// purpose. Everything around them is ordinary HTTP and worth pinning — in particular the
// ownership check on rename and delete, which is the only thing stopping one user
// removing another's second factor, and the response shape, which must never carry the
// credential id or public key.

type fakePasskeyStore struct {
	passkeys []*models.Passkey
	byID     map[uuid.UUID]*models.Passkey
	getErr   error
	listErr  error
	writeErr error
	deleted  []uuid.UUID
	updated  *models.Passkey
}

func newStore(passkeys ...*models.Passkey) *fakePasskeyStore {
	byID := make(map[uuid.UUID]*models.Passkey, len(passkeys))
	for _, pk := range passkeys {
		byID[pk.ID] = pk
	}

	return &fakePasskeyStore{passkeys: passkeys, byID: byID}
}

func (f *fakePasskeyStore) Create(context.Context, *models.Passkey) error { return f.writeErr }

func (f *fakePasskeyStore) GetByID(_ context.Context, id uuid.UUID) (*models.Passkey, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	pk, ok := f.byID[id]
	if !ok {
		return nil, passkeyRepo.ErrPasskeyNotFound
	}

	return pk, nil
}

func (f *fakePasskeyStore) GetByCredentialID(context.Context, []byte) (*models.Passkey, error) {
	return nil, passkeyRepo.ErrPasskeyNotFound
}

func (f *fakePasskeyStore) ListByUserID(_ context.Context, userID uuid.UUID) ([]*models.Passkey, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	var mine []*models.Passkey
	for _, pk := range f.passkeys {
		if pk.UserID == userID {
			mine = append(mine, pk)
		}
	}

	return mine, nil
}

func (f *fakePasskeyStore) Update(_ context.Context, passkey *models.Passkey) error {
	f.updated = passkey

	return f.writeErr
}

func (f *fakePasskeyStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deleted = append(f.deleted, id)

	return f.writeErr
}

type fakeUserLookup struct {
	user *authModels.User
	err  error
}

func (f *fakeUserLookup) GetByID(context.Context, uuid.UUID) (*authModels.User, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.user, nil
}

func newHandler(t *testing.T, store *fakePasskeyStore) *PasskeyHandler {
	t.Helper()

	user := &authModels.User{Email: "ada@example.test", DisplayName: "Ada", Role: authModels.RoleUser}
	user.ID = uuid.New()

	cfg := &config.Config{
		WebAuthnRPName:   "WhenTo",
		WebAuthnRPID:     "localhost",
		WebAuthnRPOrigin: "http://localhost:8080",
		WebAuthnTimeout:  60 * time.Second,
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc, err := service.NewPasskeyService(store, &fakeUserLookup{user: user}, cfg, &cache.NoOpCache{}, discard)
	if err != nil {
		t.Fatalf("NewPasskeyService: %v", err)
	}

	// The auth service is only reached by the finish-authentication path, which needs a
	// real attestation and is not exercised here.
	return NewPasskeyHandler(svc, nil, discard)
}

func passkeyFor(userID uuid.UUID, name string) *models.Passkey {
	return &models.Passkey{
		ID:           uuid.New(),
		UserID:       userID,
		Name:         name,
		CredentialID: []byte{0x01, 0x02, 0x03},
		PublicKey:    []byte{0xa5, 0x01, 0x02},
		CreatedAt:    time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC),
	}
}

// request builds a request with the chi URL parameter and the authenticated user, the
// way the router and the Auth middleware leave them.
func request(method, body, passkeyID string, userID string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/api/v1/passkey/"+passkeyID, nil)
	} else {
		req = httptest.NewRequest(method, "/api/v1/passkey/"+passkeyID, strings.NewReader(body))
	}

	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", passkeyID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx)
	if userID != "" {
		ctx = context.WithValue(ctx, middleware.UserIDKey, userID)
	}

	return req.WithContext(ctx)
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()

	var body envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the response is not the standard envelope: %v (%q)", err, rec.Body.String())
	}

	return body
}

func TestPasskeyEndpointsRequireAuthentication(t *testing.T) {
	handler := newHandler(t, newStore())

	endpoints := map[string]func(http.ResponseWriter, *http.Request){
		"BeginRegistration": handler.BeginRegistration,
		"List":              handler.List,
		"Rename":            handler.Rename,
		"Delete":            handler.Delete,
	}

	for name, serve := range endpoints {
		t.Run(name+" without a user", func(t *testing.T) {
			rec := httptest.NewRecorder()
			serve(rec, request(http.MethodPost, `{"name":"Key"}`, uuid.New().String(), ""))

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		})

		t.Run(name+" with a malformed user id", func(t *testing.T) {
			rec := httptest.NewRecorder()
			serve(rec, request(http.MethodPost, `{"name":"Key"}`, uuid.New().String(), "not-a-uuid"))

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestListReturnsOnlyTheUsersOwnKeys(t *testing.T) {
	mine := uuid.New()
	theirs := uuid.New()
	store := newStore(
		passkeyFor(mine, "My laptop"),
		passkeyFor(mine, "My phone"),
		passkeyFor(theirs, "Somebody else's"),
	)

	rec := httptest.NewRecorder()
	newHandler(t, store).List(rec, request(http.MethodGet, "", "", mine.String()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var listed []models.PasskeyResponse
	if err := json.Unmarshal(decode(t, rec).Data, &listed); err != nil {
		t.Fatalf("the payload is not a list of PasskeyResponse: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("got %d passkeys, want the user's own two", len(listed))
	}
	for _, pk := range listed {
		if pk.Name == "Somebody else's" {
			t.Error("another user's passkey was listed")
		}
	}

	// The response carries only the id, name and timestamps. The credential id and
	// public key identify the credential across sites and have no business in a
	// settings page payload.
	raw := rec.Body.String()
	for _, leaked := range []string{"credential_id", "public_key", "sign_count", "aaguid", "transports"} {
		if strings.Contains(raw, leaked) {
			t.Errorf("the response exposes %q: %s", leaked, raw)
		}
	}
}

func TestListWithNoPasskeys(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler(t, newStore()).List(rec, request(http.MethodGet, "", "", uuid.New().String()))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// An empty array rather than null: the settings page maps over this without a guard.
	var listed []models.PasskeyResponse
	if err := json.Unmarshal(decode(t, rec).Data, &listed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if listed == nil {
		t.Error("the payload is null rather than an empty array")
	}
}

func TestListReportsARepositoryFailure(t *testing.T) {
	store := newStore()
	store.listErr = errors.New("connection refused")

	rec := httptest.NewRecorder()
	newHandler(t, store).List(rec, request(http.MethodGet, "", "", uuid.New().String()))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	// An empty list on failure would tell the user they have no passkeys and invite
	// them to register another, or worse, to turn the feature off.
	if body := decode(t, rec); body.Success {
		t.Error("a repository failure was reported as an empty list")
	}
}

func TestRename(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	t.Run("200 and the name is written", func(t *testing.T) {
		pk := passkeyFor(owner, "Old name")
		store := newStore(pk)

		rec := httptest.NewRecorder()
		newHandler(t, store).Rename(rec, request(http.MethodPatch, `{"name":"New name"}`, pk.ID.String(), owner.String()))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}
		if store.updated == nil || store.updated.Name != "New name" {
			t.Errorf("the new name was not written: %+v", store.updated)
		}
	})

	// The one that matters. Renaming somebody else's key is mild on its own, but the
	// check is shared with delete, where it is the difference between an inconvenience
	// and removing another account's second factor.
	t.Run("403 for another user's passkey", func(t *testing.T) {
		pk := passkeyFor(owner, "Theirs")
		store := newStore(pk)

		rec := httptest.NewRecorder()
		newHandler(t, store).Rename(rec, request(http.MethodPatch, `{"name":"Mine now"}`, pk.ID.String(), stranger.String()))

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
		if store.updated != nil {
			t.Error("the passkey was modified despite the ownership check failing")
		}
	})

	tests := []struct {
		name       string
		body       string
		passkeyID  string
		wantStatus int
	}{
		{name: "a passkey that does not exist", body: `{"name":"X"}`, passkeyID: uuid.New().String(), wantStatus: http.StatusNotFound},
		{name: "a passkey id that is not a UUID", body: `{"name":"X"}`, passkeyID: "not-a-uuid", wantStatus: http.StatusBadRequest},
		{name: "a malformed body", body: `{"name":`, passkeyID: uuid.New().String(), wantStatus: http.StatusBadRequest},
		{name: "an empty name", body: `{"name":""}`, passkeyID: uuid.New().String(), wantStatus: http.StatusBadRequest},
		{name: "no name at all", body: `{}`, passkeyID: uuid.New().String(), wantStatus: http.StatusBadRequest},
		{
			// The column is VARCHAR(100); refusing here beats a database error later.
			name: "a name past the limit", body: `{"name":"` + strings.Repeat("a", 101) + `"}`,
			passkeyID: uuid.New().String(), wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newHandler(t, newStore()).Rename(rec, request(http.MethodPatch, tt.body, tt.passkeyID, owner.String()))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (%q)", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestDelete(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	t.Run("200 and the key is removed", func(t *testing.T) {
		pk := passkeyFor(owner, "Doomed")
		store := newStore(pk)

		rec := httptest.NewRecorder()
		newHandler(t, store).Delete(rec, request(http.MethodDelete, "", pk.ID.String(), owner.String()))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
		}
		if len(store.deleted) != 1 || store.deleted[0] != pk.ID {
			t.Errorf("deleted = %v, want %v", store.deleted, pk.ID)
		}
	})

	// Deleting another user's passkey removes their second factor. If they had no
	// password set, it removes their only way in.
	t.Run("403 for another user's passkey", func(t *testing.T) {
		pk := passkeyFor(owner, "Theirs")
		store := newStore(pk)

		rec := httptest.NewRecorder()
		newHandler(t, store).Delete(rec, request(http.MethodDelete, "", pk.ID.String(), stranger.String()))

		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
		if len(store.deleted) != 0 {
			t.Error("the passkey was deleted despite the ownership check failing")
		}
	})

	t.Run("404 for a passkey that does not exist", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newHandler(t, newStore()).Delete(rec, request(http.MethodDelete, "", uuid.New().String(), owner.String()))

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("400 for a passkey id that is not a UUID", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newHandler(t, newStore()).Delete(rec, request(http.MethodDelete, "", "not-a-uuid", owner.String()))

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
	})
}

// TestBeginDiscoverableAuthenticationIsAnonymous covers the usernameless login entry
// point. It has to work without a session — that is the whole feature — and it must not
// reveal whether any particular account exists.
func TestBeginDiscoverableAuthenticationIsAnonymous(t *testing.T) {
	rec := httptest.NewRecorder()
	newHandler(t, newStore()).BeginDiscoverableAuthentication(
		rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/passkey/login/begin", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%q)", rec.Code, rec.Body.String())
	}

	var options map[string]any
	if err := json.Unmarshal(decode(t, rec).Data, &options); err != nil {
		t.Fatalf("the payload is not an options object: %v", err)
	}
	// The challenge is what binds the assertion to this attempt; without one the
	// ceremony is replayable.
	if _, ok := options["challengeId"]; !ok {
		t.Errorf("no challenge id was issued: %v", options)
	}
}

func TestBeginRegistrationNeedsAKnownUser(t *testing.T) {
	// The handler is authenticated, so a user id that resolves to nothing means the
	// account was deleted between the token being issued and this request. Only the
	// id ever reaches the handler here — the lookup is stubbed to fail — so building
	// a full user record would just be dead weight.
	userID := uuid.New()

	cfg := &config.Config{
		WebAuthnRPName:   "WhenTo",
		WebAuthnRPID:     "localhost",
		WebAuthnRPOrigin: "http://localhost:8080",
		WebAuthnTimeout:  60 * time.Second,
	}
	discard := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc, err := service.NewPasskeyService(
		newStore(), &fakeUserLookup{err: errors.New("not found")}, cfg, &cache.NoOpCache{}, discard)
	if err != nil {
		t.Fatalf("NewPasskeyService: %v", err)
	}

	rec := httptest.NewRecorder()
	NewPasskeyHandler(svc, nil, discard).BeginRegistration(
		rec, request(http.MethodPost, "", "", userID.String()))

	if rec.Code == http.StatusOK {
		t.Error("registration began for a user that does not exist")
	}
}
