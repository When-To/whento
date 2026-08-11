// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/whento/whento/internal/ics/repository"
)

// The unified feed is one bearer URL that exposes several calendars at once, so the
// ownership check here is the only thing between "add my calendars" and "add anybody's".

type fakeConfigRepo struct {
	feed        *repository.UnifiedFeed
	calendarIDs []uuid.UUID
	getErr      error

	created     *repository.UnifiedFeed
	createErr   error
	owned       bool
	ownedErr    error
	updated     []uuid.UUID
	updateErr   error
	newToken    string
	tokenErr    error
	updateCalls int
}

func (f *fakeConfigRepo) GetByUserID(context.Context, uuid.UUID) (*repository.UnifiedFeed, []uuid.UUID, error) {
	if f.getErr != nil {
		return nil, nil, f.getErr
	}

	return f.feed, f.calendarIDs, nil
}

func (f *fakeConfigRepo) Create(context.Context, uuid.UUID) (*repository.UnifiedFeed, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}

	return f.created, nil
}

func (f *fakeConfigRepo) UpdateCalendars(_ context.Context, _ uuid.UUID, calendarIDs []uuid.UUID) error {
	f.updateCalls++
	f.updated = calendarIDs

	return f.updateErr
}

func (f *fakeConfigRepo) RegenerateToken(context.Context, uuid.UUID) (string, error) {
	if f.tokenErr != nil {
		return "", f.tokenErr
	}

	return f.newToken, nil
}

func (f *fakeConfigRepo) ValidateCalendarOwnership(context.Context, uuid.UUID, []uuid.UUID) (bool, error) {
	if f.ownedErr != nil {
		return false, f.ownedErr
	}

	return f.owned, nil
}

func TestGetConfigWhenNoFeedExists(t *testing.T) {
	service := NewUnifiedFeedConfigService(&fakeConfigRepo{getErr: errors.New("no rows")})

	config, err := service.GetConfig(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}

	// A user who has never created a feed is not an error case; the settings page has to
	// render an "enable this" state rather than a failure.
	if config.Configured {
		t.Error("a user with no feed reads as configured")
	}
	if config.ICSToken != "" {
		t.Errorf("ICSToken = %q, want empty", config.ICSToken)
	}
	// An empty slice rather than nil: nil marshals to JSON null, and the frontend maps
	// over this list without a guard.
	if config.IncludedCalendarIDs == nil {
		t.Error("IncludedCalendarIDs is nil, which serialises as null rather than []")
	}
}

func TestGetConfigReturnsTheFeed(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	service := NewUnifiedFeedConfigService(&fakeConfigRepo{
		feed:        &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "abc123"},
		calendarIDs: []uuid.UUID{first, second},
	})

	config, err := service.GetConfig(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}

	if !config.Configured || config.ICSToken != "abc123" {
		t.Errorf("got %+v, want a configured feed with the token", config)
	}
	if len(config.IncludedCalendarIDs) != 2 {
		t.Fatalf("got %d calendars, want 2", len(config.IncludedCalendarIDs))
	}
	if config.IncludedCalendarIDs[0] != first.String() {
		t.Errorf("the first calendar id = %q, want %q", config.IncludedCalendarIDs[0], first)
	}
}

func TestCreateFeed(t *testing.T) {
	t.Run("creates when none exists", func(t *testing.T) {
		repo := &fakeConfigRepo{
			getErr:  errors.New("no rows"),
			created: &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "fresh-token"},
		}
		service := NewUnifiedFeedConfigService(repo)

		config, err := service.CreateFeed(context.Background(), uuid.New())
		if err != nil {
			t.Fatalf("CreateFeed: %v", err)
		}
		if !config.Configured || config.ICSToken != "fresh-token" {
			t.Errorf("got %+v", config)
		}
	})

	t.Run("refuses a second feed", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{
			feed: &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "existing"},
		})

		// Two feeds would mean two live URLs, and the settings page can only ever revoke
		// the one it knows about.
		if _, err := service.CreateFeed(context.Background(), uuid.New()); !errors.Is(err, ErrFeedAlreadyExists) {
			t.Errorf("error = %v, want ErrFeedAlreadyExists", err)
		}
	})

	t.Run("propagates a repository failure", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{
			getErr:    errors.New("no rows"),
			createErr: errors.New("connection refused"),
		})

		if _, err := service.CreateFeed(context.Background(), uuid.New()); err == nil {
			t.Error("a failed insert reported success")
		}
	})
}

func TestUpdateCalendars(t *testing.T) {
	feed := &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "token"}
	mine, theirs := uuid.New(), uuid.New()

	t.Run("accepts calendars the user owns", func(t *testing.T) {
		repo := &fakeConfigRepo{feed: feed, owned: true}
		service := NewUnifiedFeedConfigService(repo)

		if err := service.UpdateCalendars(context.Background(), uuid.New(), []string{mine.String()}); err != nil {
			t.Fatalf("UpdateCalendars: %v", err)
		}
		if len(repo.updated) != 1 || repo.updated[0] != mine {
			t.Errorf("wrote %v, want %v", repo.updated, mine)
		}
	})

	t.Run("refuses a calendar the user does not own", func(t *testing.T) {
		repo := &fakeConfigRepo{feed: feed, owned: false}
		service := NewUnifiedFeedConfigService(repo)

		err := service.UpdateCalendars(context.Background(), uuid.New(), []string{theirs.String()})
		if !errors.Is(err, ErrInvalidCalendarOwner) {
			t.Errorf("error = %v, want ErrInvalidCalendarOwner", err)
		}
		// And nothing is written: a partial update would leave the foreign calendar in
		// the feed, which is the whole exposure this check exists to prevent.
		if repo.updateCalls != 0 {
			t.Error("the feed was updated despite the ownership check failing")
		}
	})

	t.Run("an empty selection skips the ownership check", func(t *testing.T) {
		// Clearing the feed is always allowed; there is nothing to own.
		repo := &fakeConfigRepo{feed: feed, ownedErr: errors.New("must not be called")}
		service := NewUnifiedFeedConfigService(repo)

		if err := service.UpdateCalendars(context.Background(), uuid.New(), nil); err != nil {
			t.Errorf("clearing the feed failed: %v", err)
		}
		if repo.updateCalls != 1 {
			t.Error("clearing the feed did not reach the repository")
		}
	})

	t.Run("no feed", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{getErr: errors.New("no rows")})

		if err := service.UpdateCalendars(context.Background(), uuid.New(), nil); !errors.Is(err, ErrFeedNotFound) {
			t.Errorf("error = %v, want ErrFeedNotFound", err)
		}
	})

	// A malformed id is rejected, but as a bare fmt.Errorf rather than a sentinel. The
	// handler has no case for it and falls through to 500, so a client that sends a
	// typo gets "Internal server error" instead of a 400 telling it what to fix.
	// Pinned here as the current contract, not as an endorsement of it.
	t.Run("a malformed calendar id is refused without a sentinel", func(t *testing.T) {
		repo := &fakeConfigRepo{feed: feed, owned: true}
		service := NewUnifiedFeedConfigService(repo)

		err := service.UpdateCalendars(context.Background(), uuid.New(), []string{"not-a-uuid"})
		if err == nil {
			t.Fatal("a malformed calendar id was accepted")
		}
		if errors.Is(err, ErrFeedNotFound) || errors.Is(err, ErrInvalidCalendarOwner) {
			t.Errorf("error = %v, want a distinct failure", err)
		}
		if repo.updateCalls != 0 {
			t.Error("the feed was updated despite a malformed id")
		}
	})

	t.Run("propagates an ownership check failure", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{
			feed: feed, ownedErr: errors.New("connection refused"),
		})

		// A failed check must not be read as "allowed": that would be the one failure
		// mode where a database outage grants access.
		err := service.UpdateCalendars(context.Background(), uuid.New(), []string{mine.String()})
		if err == nil {
			t.Error("a failed ownership check was treated as success")
		}
	})
}

func TestRegenerateTokenService(t *testing.T) {
	t.Run("returns the new token", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{
			feed:     &repository.UnifiedFeed{ID: uuid.New(), ICSToken: "old"},
			newToken: "new-token",
		})

		token, err := service.RegenerateToken(context.Background(), uuid.New())
		if err != nil {
			t.Fatalf("RegenerateToken: %v", err)
		}
		if token != "new-token" {
			t.Errorf("token = %q, want new-token", token)
		}
	})

	t.Run("no feed", func(t *testing.T) {
		service := NewUnifiedFeedConfigService(&fakeConfigRepo{getErr: errors.New("no rows")})

		if _, err := service.RegenerateToken(context.Background(), uuid.New()); !errors.Is(err, ErrFeedNotFound) {
			t.Errorf("error = %v, want ErrFeedNotFound", err)
		}
	})
}
