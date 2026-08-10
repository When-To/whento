// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/passkey/models"
)

// The package sat at 0%: NewPasskeyService took *repository.PasskeyRepository and
// *authRepo.UserRepository, both concrete and both wrapping a pgx pool. It now takes
// PasskeyStore and UserLookup, which the concrete repositories satisfy structurally.
//
// The WebAuthn ceremonies (BeginRegistration through FinishAuthentication) need a real
// authenticator and a live *http.Request carrying signed attestation, so they are not
// reachable from a unit test. What *is* reachable is the ownership check that stands
// between a passkey and anyone who is not its owner — and that is the part where a
// mistake matters most.

var errStore = errors.New("store unavailable")

type fakePasskeyStore struct {
	byID map[uuid.UUID]*models.Passkey

	listErr   error
	getErr    error
	updateErr error
	deleteErr error

	updated *models.Passkey
	deleted uuid.UUID
}

var _ PasskeyStore = (*fakePasskeyStore)(nil)

func newFakeStore() *fakePasskeyStore {
	return &fakePasskeyStore{byID: map[uuid.UUID]*models.Passkey{}}
}

func (f *fakePasskeyStore) add(passkey *models.Passkey) *models.Passkey {
	f.byID[passkey.ID] = passkey

	return passkey
}

func (f *fakePasskeyStore) Create(_ context.Context, passkey *models.Passkey) error {
	f.add(passkey)

	return nil
}

func (f *fakePasskeyStore) GetByID(_ context.Context, id uuid.UUID) (*models.Passkey, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if passkey, ok := f.byID[id]; ok {
		return passkey, nil
	}

	return nil, ErrPasskeyNotFound
}

func (f *fakePasskeyStore) GetByCredentialID(context.Context, []byte) (*models.Passkey, error) {
	return nil, ErrPasskeyNotFound
}

func (f *fakePasskeyStore) ListByUserID(_ context.Context, userID uuid.UUID) ([]*models.Passkey, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*models.Passkey, 0)
	for _, passkey := range f.byID {
		if passkey.UserID == userID {
			out = append(out, passkey)
		}
	}

	return out, nil
}

func (f *fakePasskeyStore) Update(_ context.Context, passkey *models.Passkey) error {
	f.updated = passkey

	return f.updateErr
}

func (f *fakePasskeyStore) Delete(_ context.Context, id uuid.UUID) error {
	f.deleted = id

	return f.deleteErr
}

type fakeUserLookup struct{ user *authModels.User }

var _ UserLookup = (*fakeUserLookup)(nil)

func (f *fakeUserLookup) GetByID(context.Context, uuid.UUID) (*authModels.User, error) {
	if f.user == nil {
		return nil, ErrUserNotFound
	}

	return f.user, nil
}

func newService(store *fakePasskeyStore) *PasskeyService {
	return &PasskeyService{repo: store, userRepo: &fakeUserLookup{}}
}

func passkeyFor(owner uuid.UUID, name string) *models.Passkey {
	passkey := &models.Passkey{UserID: owner, Name: name}
	passkey.ID = uuid.New()

	return passkey
}

// TestOwnershipIsEnforced is the reason this file exists. A passkey id is a bare UUID
// in the URL, so without this check anyone who learns one could rename or delete
// somebody else's second factor — and deleting the last one drops them back to a
// password alone.
func TestOwnershipIsEnforced(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	t.Run("the owner may rename", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))

		if err := newService(store).Rename(context.Background(), passkey.ID, owner, "Work laptop"); err != nil {
			t.Fatalf("Rename: %v", err)
		}
		if store.updated == nil || store.updated.Name != "Work laptop" {
			t.Errorf("stored name = %+v, want the new one", store.updated)
		}
	})

	t.Run("a stranger may not rename", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))

		if err := newService(store).Rename(
			context.Background(), passkey.ID, stranger, "Hijacked",
		); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
		if store.updated != nil {
			t.Error("a refused rename still wrote to the store")
		}
	})

	t.Run("the owner may delete", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))

		if err := newService(store).Delete(context.Background(), passkey.ID, owner); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if store.deleted != passkey.ID {
			t.Errorf("deleted %v, want %v", store.deleted, passkey.ID)
		}
	})

	t.Run("a stranger may not delete", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))

		if err := newService(store).Delete(
			context.Background(), passkey.ID, stranger,
		); !errors.Is(err, ErrUnauthorized) {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
		if store.deleted != uuid.Nil {
			t.Error("a refused delete still reached the store")
		}
	})
}

func TestRenameAndDeleteOnAnUnknownPasskey(t *testing.T) {
	store := newFakeStore()
	service := newService(store)

	if err := service.Rename(context.Background(), uuid.New(), uuid.New(), "x"); !errors.Is(err, ErrPasskeyNotFound) {
		t.Errorf("Rename error = %v, want ErrPasskeyNotFound", err)
	}
	if err := service.Delete(context.Background(), uuid.New(), uuid.New()); !errors.Is(err, ErrPasskeyNotFound) {
		t.Errorf("Delete error = %v, want ErrPasskeyNotFound", err)
	}
}

func TestRenameAndDeletePropagateStoreFailures(t *testing.T) {
	owner := uuid.New()

	t.Run("the lookup fails", func(t *testing.T) {
		store := newFakeStore()
		store.getErr = errStore

		if err := newService(store).Rename(context.Background(), uuid.New(), owner, "x"); !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})

	t.Run("the write fails", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))
		store.updateErr = errStore

		if err := newService(store).Rename(context.Background(), passkey.ID, owner, "x"); !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})

	t.Run("the delete fails", func(t *testing.T) {
		store := newFakeStore()
		passkey := store.add(passkeyFor(owner, "Laptop"))
		store.deleteErr = errStore

		if err := newService(store).Delete(context.Background(), passkey.ID, owner); !errors.Is(err, errStore) {
			t.Errorf("error = %v, want the store failure", err)
		}
	})
}

// TestListReturnsOnlyTheCallersPasskeys guards the other direction of the same concern:
// the list is scoped by user, so one account never sees another's devices.
func TestListReturnsOnlyTheCallersPasskeys(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	store := newFakeStore()
	store.add(passkeyFor(owner, "Laptop"))
	store.add(passkeyFor(owner, "Phone"))
	store.add(passkeyFor(stranger, "Someone else's key"))

	mine, err := newService(store).List(context.Background(), owner)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(mine) != 2 {
		t.Fatalf("got %d passkeys, want 2", len(mine))
	}
	for _, passkey := range mine {
		if passkey.UserID != owner {
			t.Errorf("the list included a passkey owned by %v", passkey.UserID)
		}
	}

	empty, err := newService(store).List(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("a user with no passkeys got %d", len(empty))
	}
}

func TestListPropagatesAStoreFailure(t *testing.T) {
	store := newFakeStore()
	store.listErr = errStore

	if _, err := newService(store).List(context.Background(), uuid.New()); !errors.Is(err, errStore) {
		t.Errorf("error = %v, want the store failure", err)
	}
}
