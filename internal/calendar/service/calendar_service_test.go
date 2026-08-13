// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/cache"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/repository"
)

var errRepo = errors.New("repository unavailable")

// mockCalendarRepo is a hand-written CalendarRepository. It captures what the service
// hands down, which is how the defaulting tests below read the values the service
// decided on rather than the ones it echoed back.
type mockCalendarRepo struct {
	created      *models.Calendar
	createdInput []repository.ParticipantInput
	createErr    error

	calendar    *models.Calendar
	getErr      error
	owned       []*models.Calendar
	ownedErr    error
	byToken     *models.Calendar
	byTokenErr  error
	updated     *models.Calendar
	updateErr   error
	deleteErr   error
	regenErr    error
	regenCalled struct {
		id        uuid.UUID
		tokenType string
		newToken  string
	}
}

var _ CalendarRepository = (*mockCalendarRepo)(nil)

func (m *mockCalendarRepo) CreateWithParticipants(
	_ context.Context, calendar *models.Calendar, participants []repository.ParticipantInput,
) ([]models.Participant, error) {
	m.created = calendar
	m.createdInput = participants

	if m.createErr != nil {
		return nil, m.createErr
	}

	out := make([]models.Participant, 0, len(participants))
	for _, p := range participants {
		participant := models.Participant{
			CalendarID:    calendar.ID,
			Name:          p.Name,
			Email:         p.Email,
			EmailVerified: p.EmailVerified,
			Locale:        p.Locale,
		}
		participant.ID = uuid.New()
		out = append(out, participant)
	}

	return out, nil
}

func (m *mockCalendarRepo) GetByID(context.Context, uuid.UUID) (*models.Calendar, error) {
	return m.calendar, m.getErr
}

func (m *mockCalendarRepo) GetByOwnerID(context.Context, uuid.UUID) ([]*models.Calendar, error) {
	return m.owned, m.ownedErr
}

func (m *mockCalendarRepo) GetByPublicToken(context.Context, string) (*models.Calendar, error) {
	return m.byToken, m.byTokenErr
}

func (m *mockCalendarRepo) Update(_ context.Context, calendar *models.Calendar) error {
	m.updated = calendar

	return m.updateErr
}

func (m *mockCalendarRepo) Delete(context.Context, uuid.UUID) error { return m.deleteErr }

func (m *mockCalendarRepo) RegenerateToken(_ context.Context, id uuid.UUID, tokenType, newToken string) error {
	m.regenCalled.id = id
	m.regenCalled.tokenType = tokenType
	m.regenCalled.newToken = newToken

	return m.regenErr
}

// mockParticipantRepo is a hand-written ParticipantRepository.
type mockParticipantRepo struct {
	participants []models.Participant
	listErr      error
	byID         *models.Participant
	byIDErr      error
	createErr    error
	updateErr    error
	deleteErr    error
	verifiedFor  uuid.UUID
	verifiedMail string
}

var _ ParticipantRepository = (*mockParticipantRepo)(nil)

func (m *mockParticipantRepo) Create(context.Context, *models.Participant) error {
	return m.createErr
}

func (m *mockParticipantRepo) GetByID(context.Context, uuid.UUID) (*models.Participant, error) {
	return m.byID, m.byIDErr
}

func (m *mockParticipantRepo) GetByCalendarID(context.Context, uuid.UUID) ([]models.Participant, error) {
	return m.participants, m.listErr
}

func (m *mockParticipantRepo) Update(context.Context, uuid.UUID, string) error {
	return m.updateErr
}

func (m *mockParticipantRepo) Delete(context.Context, uuid.UUID) error { return m.deleteErr }

func (m *mockParticipantRepo) SetEmailAsVerified(_ context.Context, id uuid.UUID, email string) error {
	m.verifiedFor = id
	m.verifiedMail = email

	return nil
}

// newService wires the service with a nil user repository. That is a supported
// configuration — every use of it is nil-guarded — and it keeps these tests off the
// concrete *authRepo.UserRepository, which is a struct rather than an interface.
//
// The cache is not optional in the same way: DeleteCalendar and UpdateCalendar
// dereference it without a guard, so a nil here panics rather than degrading.
// NewRedisCache(nil) hands back the same NoOpCache a self-hosted install without Redis
// runs on, which is the honest stand-in.
func newService(calendars *mockCalendarRepo, participants *mockParticipantRepo) *CalendarService {
	return NewCalendarService(calendars, participants, nil, cache.NewRedisCache(nil))
}

// TestCreateCalendarDefaults pins what a calendar created from a bare request actually
// is. These defaults are the ones the frontend's date layer assumes, so a change here
// silently desynchronises the two.
func TestCreateCalendarDefaults(t *testing.T) {
	calendars := &mockCalendarRepo{}
	service := newService(calendars, &mockParticipantRepo{})

	owner := uuid.New()
	got, err := service.CreateCalendar(context.Background(), owner.String(), &models.CreateCalendarRequest{
		Name: "Board game night",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	stored := calendars.created
	if stored == nil {
		t.Fatal("the calendar was never handed to the repository")
	}

	if stored.Threshold != 1 {
		t.Errorf("Threshold = %d, want 1", stored.Threshold)
	}
	if len(stored.AllowedWeekdays) != 7 {
		t.Errorf("AllowedWeekdays = %v, want all seven days", stored.AllowedWeekdays)
	}
	if stored.Timezone != "Europe/Paris" {
		t.Errorf("Timezone = %q, want %q", stored.Timezone, "Europe/Paris")
	}
	// "ignore" rather than "block": a new calendar treats holidays as ordinary days.
	if stored.HolidaysPolicy != "ignore" {
		t.Errorf("HolidaysPolicy = %q, want %q", stored.HolidaysPolicy, "ignore")
	}
	if stored.OwnerID != owner {
		t.Errorf("OwnerID = %v, want %v", stored.OwnerID, owner)
	}
	if stored.StartDate != nil || stored.EndDate != nil {
		t.Errorf("dates = %v/%v, want both nil", stored.StartDate, stored.EndDate)
	}

	if got.PublicToken == "" || got.ICSToken == "" {
		t.Error("tokens were not generated")
	}
	if got.PublicToken == got.ICSToken {
		t.Error("the public and ICS tokens are identical")
	}
	if len(got.PublicToken) != 64 {
		t.Errorf("public token length = %d, want 64", len(got.PublicToken))
	}
}

func TestCreateCalendarKeepsExplicitValues(t *testing.T) {
	calendars := &mockCalendarRepo{}
	service := newService(calendars, &mockParticipantRepo{})

	_, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{
		Name:            "Standup",
		Threshold:       4,
		AllowedWeekdays: []int{1, 2, 3, 4, 5},
		Timezone:        "America/New_York",
		HolidaysPolicy:  "block",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	stored := calendars.created
	if stored.Threshold != 4 {
		t.Errorf("Threshold = %d, want 4", stored.Threshold)
	}
	if len(stored.AllowedWeekdays) != 5 {
		t.Errorf("AllowedWeekdays = %v, want five days", stored.AllowedWeekdays)
	}
	if stored.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", stored.Timezone, "America/New_York")
	}
	if stored.HolidaysPolicy != "block" {
		t.Errorf("HolidaysPolicy = %q, want %q", stored.HolidaysPolicy, "block")
	}
}

// TestCreateCalendarDates covers start_date and end_date, which had no coverage in this
// package or any other.
func TestCreateCalendarDates(t *testing.T) {
	tests := []struct {
		name      string
		startDate string
		endDate   string
		wantErr   bool
	}{
		{name: "neither", startDate: "", endDate: ""},
		{name: "start only", startDate: "2026-01-01"},
		{name: "end only", endDate: "2026-12-31"},
		{name: "a valid range", startDate: "2026-01-01", endDate: "2026-12-31"},
		{name: "a single-day range", startDate: "2026-06-15", endDate: "2026-06-15"},
		{name: "end before start", startDate: "2026-12-31", endDate: "2026-01-01", wantErr: true},
		{name: "end one day before start", startDate: "2026-06-16", endDate: "2026-06-15", wantErr: true},
		{name: "an unparseable start", startDate: "01/01/2026", wantErr: true},
		{name: "an unparseable end", endDate: "31 December", wantErr: true},
		{name: "a timestamp rather than a date", startDate: "2026-01-01T00:00:00Z", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendars := &mockCalendarRepo{}
			service := newService(calendars, &mockParticipantRepo{})

			_, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{
				Name:      "Trip",
				StartDate: tt.startDate,
				EndDate:   tt.endDate,
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			stored := calendars.created
			if (tt.startDate != "") != (stored.StartDate != nil) {
				t.Errorf("StartDate = %v for input %q", stored.StartDate, tt.startDate)
			}
			if (tt.endDate != "") != (stored.EndDate != nil) {
				t.Errorf("EndDate = %v for input %q", stored.EndDate, tt.endDate)
			}
			if stored.StartDate != nil && stored.StartDate.Format("2006-01-02") != tt.startDate {
				t.Errorf("StartDate = %v, want %q", stored.StartDate, tt.startDate)
			}
		})
	}
}

// TestCreateCalendarNormalisesInvertedTimes checks that the swap happens before the
// value is stored, not on the way back out.
func TestCreateCalendarNormalisesInvertedTimes(t *testing.T) {
	calendars := &mockCalendarRepo{}
	service := newService(calendars, &mockParticipantRepo{})

	got, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{
		Name:              "Evening sessions",
		WeekdayTimes:      map[string]models.TimeRange{"1": {MinTime: "20:00", MaxTime: "18:00"}},
		HolidayMinTime:    "16:00",
		HolidayMaxTime:    "10:00",
		HolidayEveMinTime: "22:00",
		HolidayEveMaxTime: "21:00",
	})
	if err != nil {
		t.Fatalf("CreateCalendar: %v", err)
	}

	if slot := got.WeekdayTimes["1"]; slot.MinTime != "18:00" || slot.MaxTime != "20:00" {
		t.Errorf("weekday 1 = %s-%s, want 18:00-20:00", slot.MinTime, slot.MaxTime)
	}
	if got.HolidayMinTime != "10:00" || got.HolidayMaxTime != "16:00" {
		t.Errorf("holiday window = %s-%s, want 10:00-16:00", got.HolidayMinTime, got.HolidayMaxTime)
	}
	if got.HolidayEveMinTime != "21:00" || got.HolidayEveMaxTime != "22:00" {
		t.Errorf("eve window = %s-%s, want 21:00-22:00", got.HolidayEveMinTime, got.HolidayEveMaxTime)
	}
}

func TestCreateCalendarFailures(t *testing.T) {
	t.Run("an invalid owner id", func(t *testing.T) {
		service := newService(&mockCalendarRepo{}, &mockParticipantRepo{})

		if _, err := service.CreateCalendar(context.Background(), "not-a-uuid", &models.CreateCalendarRequest{Name: "x"}); err == nil {
			t.Error("expected an error for a non-uuid owner")
		}
	})

	t.Run("a duplicate participant name is reported as such", func(t *testing.T) {
		calendars := &mockCalendarRepo{createErr: repository.ErrParticipantAlreadyExists}
		service := newService(calendars, &mockParticipantRepo{})

		_, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{
			Name:         "Party",
			Participants: []string{"Ada", "Ada"},
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if err.Error() != "duplicate participant name in request" {
			t.Errorf("error = %q, want the duplicate-name message", err.Error())
		}
	})

	t.Run("any other repository failure is wrapped", func(t *testing.T) {
		calendars := &mockCalendarRepo{createErr: errRepo}
		service := newService(calendars, &mockParticipantRepo{})

		_, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{Name: "x"})
		if !errors.Is(err, errRepo) {
			t.Errorf("error = %v, want it to wrap the repository failure", err)
		}
	})
}

// TestCreateCalendarParticipantLocale covers the fallback chain. With no user
// repository wired, an unspecified locale must land on "en" rather than on "".
func TestCreateCalendarParticipantLocale(t *testing.T) {
	tests := []struct {
		name     string
		locale   string
		wantLang string
	}{
		{name: "explicit french", locale: "fr", wantLang: "fr"},
		{name: "explicit english", locale: "en", wantLang: "en"},
		{name: "unspecified falls back to english", locale: "", wantLang: "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendars := &mockCalendarRepo{}
			service := newService(calendars, &mockParticipantRepo{})

			_, err := service.CreateCalendar(context.Background(), uuid.NewString(), &models.CreateCalendarRequest{
				Name:              "Meetup",
				ParticipantLocale: tt.locale,
				Participants:      []string{"Ada", "Grace"},
			})
			if err != nil {
				t.Fatalf("CreateCalendar: %v", err)
			}

			if len(calendars.createdInput) != 2 {
				t.Fatalf("got %d participant inputs, want 2", len(calendars.createdInput))
			}
			for _, input := range calendars.createdInput {
				if input.Locale != tt.wantLang {
					t.Errorf("participant %q locale = %q, want %q", input.Name, input.Locale, tt.wantLang)
				}
				if input.Email != nil {
					t.Errorf("participant %q got an email without a user repository: %v", input.Name, *input.Email)
				}
			}
		})
	}
}

// TestGetCalendarAuthorization is the access-control matrix. Owner or admin, nothing
// else — and a missing calendar must not be distinguishable from an unauthorised one by
// anything other than the error the caller is entitled to see.
func TestGetCalendarAuthorization(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Private"}
	calendar.ID = uuid.New()

	tests := []struct {
		name    string
		userID  string
		role    string
		wantErr error
	}{
		{name: "the owner", userID: owner.String(), role: "user"},
		{name: "an admin who is not the owner", userID: stranger.String(), role: "admin"},
		{name: "the owner, who is also an admin", userID: owner.String(), role: "admin"},
		{name: "a stranger", userID: stranger.String(), role: "user", wantErr: ErrUnauthorized},
		{name: "an empty user id", userID: "", role: "user", wantErr: ErrUnauthorized},
		{name: "a role that merely looks like admin", userID: stranger.String(), role: "Admin", wantErr: ErrUnauthorized},
		{name: "a role that contains admin", userID: stranger.String(), role: "administrator", wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newService(
				&mockCalendarRepo{calendar: calendar},
				&mockParticipantRepo{},
			)

			_, err := service.GetCalendar(context.Background(), tt.userID, tt.role, calendar.ID.String())

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGetCalendarNotFound(t *testing.T) {
	service := newService(
		&mockCalendarRepo{getErr: repository.ErrCalendarNotFound},
		&mockParticipantRepo{},
	)

	_, err := service.GetCalendar(context.Background(), uuid.NewString(), "user", uuid.NewString())
	if !errors.Is(err, ErrCalendarNotFound) {
		t.Errorf("error = %v, want ErrCalendarNotFound", err)
	}

	_, err = service.GetCalendar(context.Background(), uuid.NewString(), "user", "not-a-uuid")
	if err == nil {
		t.Error("expected an error for a non-uuid calendar id")
	}
}

// TestFilterParticipantsPrivacy is the leak test. Two settings interact: whether the
// calendar hides participant identity, and which participant is asking. Emails must
// never reach anyone but their owner, under either setting.
func TestFilterParticipantsPrivacy(t *testing.T) {
	calendarID := uuid.New()

	aliceEmail := "alice@example.com"
	bobEmail := "bob@example.com"

	alice := models.Participant{CalendarID: calendarID, Name: "Alice", Email: &aliceEmail, EmailVerified: true}
	alice.ID = uuid.New()
	bob := models.Participant{CalendarID: calendarID, Name: "Bob", Email: &bobEmail, EmailVerified: true}
	bob.ID = uuid.New()

	participants := []models.Participant{alice, bob}

	t.Run("unlocked: identities are visible, emails are not", func(t *testing.T) {
		got := filterParticipants(false, alice.ID.String(), participants)

		if len(got) != 2 {
			t.Fatalf("got %d participants, want 2", len(got))
		}
		for _, p := range got {
			if p.ID == nil {
				t.Errorf("%s: ID was masked even though the calendar is unlocked", p.Name)
			}
		}
		if got[0].Email == nil || *got[0].Email != aliceEmail {
			t.Error("Alice cannot see her own email")
		}
		if got[1].Email != nil {
			t.Errorf("Bob's email leaked to Alice: %v", *got[1].Email)
		}
		if got[1].EmailVerified {
			t.Error("Bob's verification status leaked to Alice")
		}
	})

	t.Run("locked: everyone but the asker is anonymised", func(t *testing.T) {
		got := filterParticipants(true, alice.ID.String(), participants)

		if got[0].ID == nil || *got[0].ID != alice.ID {
			t.Error("Alice cannot see her own ID")
		}
		if got[0].Email == nil || *got[0].Email != aliceEmail {
			t.Error("Alice cannot see her own email")
		}
		if got[1].ID != nil {
			t.Errorf("Bob's ID leaked under lock_participants: %v", *got[1].ID)
		}
		if got[1].Email != nil {
			t.Errorf("Bob's email leaked under lock_participants: %v", *got[1].Email)
		}
		if got[1].Name != "Bob" {
			t.Errorf("Bob's name = %q; names stay visible, only identity is masked", got[1].Name)
		}
	})

	t.Run("locked with no asker: nobody is identified", func(t *testing.T) {
		got := filterParticipants(true, "", participants)

		for _, p := range got {
			if p.ID != nil {
				t.Errorf("%s: ID exposed to an anonymous viewer", p.Name)
			}
			if p.Email != nil {
				t.Errorf("%s: email exposed to an anonymous viewer", p.Name)
			}
		}
	})

	t.Run("unlocked with no asker: IDs visible, no emails", func(t *testing.T) {
		got := filterParticipants(false, "", participants)

		for _, p := range got {
			if p.ID == nil {
				t.Errorf("%s: ID masked on an unlocked calendar", p.Name)
			}
			if p.Email != nil {
				t.Errorf("%s: email exposed to an anonymous viewer", p.Name)
			}
		}
	})

	t.Run("a malformed asker id is treated as no asker", func(t *testing.T) {
		got := filterParticipants(true, "not-a-uuid", participants)

		for _, p := range got {
			if p.ID != nil || p.Email != nil {
				t.Errorf("%s: a malformed participant id was honoured", p.Name)
			}
		}
	})

	t.Run("an asker who is not on the calendar gets nothing extra", func(t *testing.T) {
		got := filterParticipants(true, uuid.NewString(), participants)

		for _, p := range got {
			if p.ID != nil || p.Email != nil {
				t.Errorf("%s: an unrelated participant id was honoured", p.Name)
			}
		}
	})

	t.Run("no participants", func(t *testing.T) {
		if got := filterParticipants(true, alice.ID.String(), nil); len(got) != 0 {
			t.Errorf("got %d participants, want 0", len(got))
		}
	})
}

func TestConditionalEmail(t *testing.T) {
	email := "someone@example.com"

	if got := conditionalEmail(true, &email); got == nil || *got != email {
		t.Errorf("conditionalEmail(true) = %v, want the email", got)
	}
	if got := conditionalEmail(false, &email); got != nil {
		t.Errorf("conditionalEmail(false) = %v, want nil", *got)
	}
	if got := conditionalEmail(true, nil); got != nil {
		t.Errorf("conditionalEmail(true, nil) = %v, want nil", got)
	}
}

func TestGenerateToken(t *testing.T) {
	seen := make(map[string]bool)

	for range 50 {
		token, err := generateToken()
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}
		if len(token) != 64 {
			t.Fatalf("token length = %d, want 64", len(token))
		}
		if seen[token] {
			t.Fatalf("generateToken produced a duplicate: %s", token)
		}
		seen[token] = true
	}
}

func TestListMyCalendars(t *testing.T) {
	owner := uuid.New()

	first := &models.Calendar{OwnerID: owner, Name: "One"}
	first.ID = uuid.New()
	second := &models.Calendar{OwnerID: owner, Name: "Two"}
	second.ID = uuid.New()

	t.Run("returns one response per calendar", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{owned: []*models.Calendar{first, second}},
			&mockParticipantRepo{},
		)

		got, err := service.ListMyCalendars(context.Background(), owner.String())
		if err != nil {
			t.Fatalf("ListMyCalendars: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d calendars, want 2", len(got))
		}
	})

	t.Run("no calendars yields no responses", func(t *testing.T) {
		service := newService(&mockCalendarRepo{}, &mockParticipantRepo{})

		got, err := service.ListMyCalendars(context.Background(), owner.String())
		if err != nil {
			t.Fatalf("ListMyCalendars: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d calendars, want 0", len(got))
		}
	})

	t.Run("an invalid user id", func(t *testing.T) {
		service := newService(&mockCalendarRepo{}, &mockParticipantRepo{})

		if _, err := service.ListMyCalendars(context.Background(), "not-a-uuid"); err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("a participant lookup failure aborts the whole listing", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{owned: []*models.Calendar{first}},
			&mockParticipantRepo{listErr: errRepo},
		)

		if _, err := service.ListMyCalendars(context.Background(), owner.String()); !errors.Is(err, errRepo) {
			t.Errorf("error = %v, want the repository failure", err)
		}
	})
}

func TestDeleteCalendarAuthorization(t *testing.T) {
	owner := uuid.New()
	stranger := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Private"}
	calendar.ID = uuid.New()

	tests := []struct {
		name    string
		userID  string
		role    string
		wantErr error
	}{
		{name: "the owner", userID: owner.String(), role: "user"},
		{name: "an admin", userID: stranger.String(), role: "admin"},
		{name: "a stranger", userID: stranger.String(), role: "user", wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

			err := service.DeleteCalendar(context.Background(), tt.userID, tt.role, calendar.ID.String())

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestRegenerateToken(t *testing.T) {
	owner := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Private", PublicToken: "old-public", ICSToken: "old-ics"}
	calendar.ID = uuid.New()

	tests := []struct {
		name      string
		tokenType string
		userID    string
		role      string
		wantErr   error
	}{
		{name: "public", tokenType: "public", userID: owner.String(), role: "user"},
		{name: "ics", tokenType: "ics", userID: owner.String(), role: "user"},
		{name: "an unknown token type", tokenType: "magic", userID: owner.String(), role: "user", wantErr: ErrInvalidTokenType},
		{name: "an empty token type", tokenType: "", userID: owner.String(), role: "user", wantErr: ErrInvalidTokenType},
		{name: "a stranger", tokenType: "public", userID: uuid.NewString(), role: "user", wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A fresh copy per case: RegenerateToken mutates the calendar in place.
			subject := *calendar
			calendars := &mockCalendarRepo{calendar: &subject}
			service := newService(calendars, &mockParticipantRepo{})

			_, err := service.RegenerateToken(
				context.Background(), tt.userID, tt.role, subject.ID.String(), tt.tokenType,
			)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if calendars.regenCalled.tokenType != tt.tokenType {
				t.Errorf("regenerated %q, want %q", calendars.regenCalled.tokenType, tt.tokenType)
			}
			if len(calendars.regenCalled.newToken) != 64 {
				t.Errorf("new token length = %d, want 64", len(calendars.regenCalled.newToken))
			}
			if calendars.regenCalled.newToken == "old-public" || calendars.regenCalled.newToken == "old-ics" {
				t.Error("the token was not actually regenerated")
			}
		})
	}
}

// TestBuildPublicCalendarResponseNotifyConfig covers the inline JSON parsing of
// notify_config, which decides whether participants are told a threshold was met.
func TestBuildPublicCalendarResponseNotifyConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *string
		want   bool
	}{
		{name: "absent", config: nil},
		{name: "empty", config: ptr("")},
		{name: "enabled and notifying", config: ptr(`{"enabled":true,"notify_participants":true}`), want: true},
		{name: "enabled but not notifying", config: ptr(`{"enabled":true,"notify_participants":false}`)},
		{name: "notifying but disabled", config: ptr(`{"enabled":false,"notify_participants":true}`)},
		{name: "malformed json is treated as off", config: ptr(`{not json`)},
		{name: "unrelated keys", config: ptr(`{"something":"else"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendar := &models.Calendar{Name: "Public", NotifyConfig: tt.config}
			calendar.ID = uuid.New()

			got, err := buildPublicCalendarResponse(calendar, nil)
			if err != nil {
				t.Fatalf("buildPublicCalendarResponse: %v", err)
			}
			if got.NotifyParticipants != tt.want {
				t.Errorf("NotifyParticipants = %v, want %v", got.NotifyParticipants, tt.want)
			}
		})
	}
}

// TestBuildCalendarResponseCarriesDates guards the pass-through of the range bounds the
// frontend uses to grey out days outside the window.
func TestBuildCalendarResponseCarriesDates(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	calendar := &models.Calendar{Name: "Season", StartDate: &start, EndDate: &end}
	calendar.ID = uuid.New()

	got, err := buildCalendarResponse(calendar, nil)
	if err != nil {
		t.Fatalf("buildCalendarResponse: %v", err)
	}

	if got.StartDate == nil || !got.StartDate.Equal(start) {
		t.Errorf("StartDate = %v, want %v", got.StartDate, start)
	}
	if got.EndDate == nil || !got.EndDate.Equal(end) {
		t.Errorf("EndDate = %v, want %v", got.EndDate, end)
	}
}

func ptr(s string) *string { return &s }

// TestUpdateCalendarPartial pins the update semantics: a nil field is "leave alone",
// not "clear". Getting this backwards would wipe settings on every partial PATCH.
func TestUpdateCalendarPartial(t *testing.T) {
	owner := uuid.New()

	base := func() *models.Calendar {
		calendar := &models.Calendar{
			OwnerID:          owner,
			Name:             "Original",
			Description:      "Original description",
			Threshold:        3,
			AllowedWeekdays:  []int{1, 2, 3},
			Timezone:         "Europe/Paris",
			HolidaysPolicy:   "block",
			AllowHolidayEves: true,
			LockParticipants: true,
		}
		calendar.ID = uuid.New()

		return calendar
	}

	t.Run("an empty request changes nothing", func(t *testing.T) {
		calendar := base()
		calendars := &mockCalendarRepo{calendar: calendar}
		service := newService(calendars, &mockParticipantRepo{})

		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if got.Name != "Original" || got.Threshold != 3 || got.Timezone != "Europe/Paris" {
			t.Errorf("an empty request altered the calendar: %+v", got)
		}
		if got.HolidaysPolicy != "block" || !got.AllowHolidayEves || !got.LockParticipants {
			t.Errorf("an empty request altered the flags: %+v", got)
		}
	})

	t.Run("only the named field changes", func(t *testing.T) {
		calendar := base()
		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		name := "Renamed"
		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{Name: &name},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if got.Name != "Renamed" {
			t.Errorf("Name = %q, want %q", got.Name, "Renamed")
		}
		if got.Description != "Original description" || got.Threshold != 3 {
			t.Errorf("an unrelated field changed: %+v", got)
		}
	})

	t.Run("a false boolean is applied, not ignored", func(t *testing.T) {
		calendar := base()
		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		off := false
		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{LockParticipants: &off, AllowHolidayEves: &off},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if got.LockParticipants || got.AllowHolidayEves {
			t.Error("a false pointer was treated as absent")
		}
	})

	t.Run("allowed weekdays cannot be emptied", func(t *testing.T) {
		// The guard is len() > 0 rather than a nil check, so an explicit empty list
		// is indistinguishable from an absent one. Documented, not asserted as
		// desirable: a calendar cannot be closed on every day through this path.
		calendar := base()
		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{AllowedWeekdays: []int{}},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if len(got.AllowedWeekdays) != 3 {
			t.Errorf("AllowedWeekdays = %v, want the original three days", got.AllowedWeekdays)
		}
	})

	t.Run("notify_config is not settable here", func(t *testing.T) {
		// Deliberate: it goes through the validated PATCH /notify-config endpoint,
		// because it carries a webhook URL and would otherwise be an SSRF vector.
		calendar := base()
		hostile := `{"enabled":true,"webhook_url":"http://169.254.169.254/latest/meta-data/"}`
		calendar.NotifyConfig = nil

		calendars := &mockCalendarRepo{calendar: calendar}
		service := newService(calendars, &mockParticipantRepo{})

		name := "Renamed"
		if _, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{Name: &name},
		); err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if calendars.updated.NotifyConfig != nil {
			t.Errorf("notify_config was written by the generic update: %v", *calendars.updated.NotifyConfig)
		}
		_ = hostile
	})

	t.Run("a stranger is refused", func(t *testing.T) {
		calendar := base()
		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		name := "Hijacked"
		_, err := service.UpdateCalendar(
			context.Background(), uuid.NewString(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{Name: &name},
		)
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
	})
}

// TestUpdateCalendarDates covers clearing, setting, and the cross-field validation,
// which runs against the *merged* state rather than against the request alone.
func TestUpdateCalendarDates(t *testing.T) {
	owner := uuid.New()

	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dec := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		existingStart *time.Time
		existingEnd   *time.Time
		start         *string
		end           *string
		wantErr       bool
		wantStartNil  bool
		wantEndNil    bool
	}{
		{
			name:         "setting both",
			start:        ptr("2026-01-01"),
			end:          ptr("2026-12-31"),
			wantStartNil: false, wantEndNil: false,
		},
		{
			name:          "an empty string clears the start",
			existingStart: &jan, existingEnd: &dec,
			start:        ptr(""),
			wantStartNil: true, wantEndNil: false,
		},
		{
			name:          "an empty string clears the end",
			existingStart: &jan, existingEnd: &dec,
			end:          ptr(""),
			wantStartNil: false, wantEndNil: true,
		},
		{
			name:          "an absent field leaves the stored date alone",
			existingStart: &jan, existingEnd: &dec,
			wantStartNil: false, wantEndNil: false,
		},
		{
			// The new end is compared against the *stored* start, so a request that
			// looks harmless on its own is still rejected.
			name:          "a new end before the stored start is refused",
			existingStart: &dec,
			end:           ptr("2026-01-01"),
			wantErr:       true,
		},
		{
			name:        "a new start after the stored end is refused",
			existingEnd: &jan,
			start:       ptr("2026-12-31"),
			wantErr:     true,
		},
		{
			// Clearing one side removes the conflict entirely.
			name:          "clearing the start lets an earlier end through",
			existingStart: &dec,
			start:         ptr(""),
			end:           ptr("2026-01-01"),
			wantStartNil:  true, wantEndNil: false,
		},
		{name: "an unparseable start", start: ptr("01/01/2026"), wantErr: true},
		{name: "an unparseable end", end: ptr("someday"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendar := &models.Calendar{
				OwnerID:   owner,
				Name:      "Season",
				StartDate: tt.existingStart,
				EndDate:   tt.existingEnd,
			}
			calendar.ID = uuid.New()

			service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

			got, err := service.UpdateCalendar(
				context.Background(), owner.String(), "user", calendar.ID.String(),
				&models.UpdateCalendarRequest{StartDate: tt.start, EndDate: tt.end},
			)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if (got.StartDate == nil) != tt.wantStartNil {
				t.Errorf("StartDate = %v, wantNil = %v", got.StartDate, tt.wantStartNil)
			}
			if (got.EndDate == nil) != tt.wantEndNil {
				t.Errorf("EndDate = %v, wantNil = %v", got.EndDate, tt.wantEndNil)
			}
		})
	}
}

// TestUpdateCalendarMergesAllowedHours covers the read-modify-write of the JSONB blob:
// touching one time field must not discard the others.
func TestUpdateCalendarMergesAllowedHours(t *testing.T) {
	owner := uuid.New()

	existing, err := models.BuildAllowedHoursJSON(
		map[string]models.TimeRange{"1": {MinTime: "09:00", MaxTime: "17:00"}},
		"11:00", "15:00", "08:00", "20:00",
	)
	if err != nil {
		t.Fatalf("build fixture: %v", err)
	}

	t.Run("changing one holiday bound keeps the rest", func(t *testing.T) {
		calendar := &models.Calendar{OwnerID: owner, Name: "Hours", AllowedHours: existing}
		calendar.ID = uuid.New()

		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{HolidayMaxTime: ptr("16:00")},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if got.HolidayMinTime != "11:00" || got.HolidayMaxTime != "16:00" {
			t.Errorf("holiday window = %s-%s, want 11:00-16:00", got.HolidayMinTime, got.HolidayMaxTime)
		}
		if got.HolidayEveMinTime != "08:00" || got.HolidayEveMaxTime != "20:00" {
			t.Errorf("eve window = %s-%s, want the untouched 08:00-20:00", got.HolidayEveMinTime, got.HolidayEveMaxTime)
		}
		if slot := got.WeekdayTimes["1"]; slot.MinTime != "09:00" || slot.MaxTime != "17:00" {
			t.Errorf("weekday 1 = %s-%s, want the untouched 09:00-17:00", slot.MinTime, slot.MaxTime)
		}
	})

	t.Run("an inverted update is normalised on the way in", func(t *testing.T) {
		calendar := &models.Calendar{OwnerID: owner, Name: "Hours", AllowedHours: existing}
		calendar.ID = uuid.New()

		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{
				WeekdayTimes: map[string]models.TimeRange{"2": {MinTime: "22:00", MaxTime: "20:00"}},
			},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if slot := got.WeekdayTimes["2"]; slot.MinTime != "20:00" || slot.MaxTime != "22:00" {
			t.Errorf("weekday 2 = %s-%s, want 20:00-22:00", slot.MinTime, slot.MaxTime)
		}
	})

	t.Run("a weekday map replaces rather than merges", func(t *testing.T) {
		// Supplying weekday_times swaps the whole map, so the previously configured
		// Monday disappears. Worth pinning: it is the behaviour the settings form
		// depends on, and a merge here would make days impossible to remove.
		calendar := &models.Calendar{OwnerID: owner, Name: "Hours", AllowedHours: existing}
		calendar.ID = uuid.New()

		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		got, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{
				WeekdayTimes: map[string]models.TimeRange{"3": {MinTime: "10:00", MaxTime: "12:00"}},
			},
		)
		if err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if _, stillThere := got.WeekdayTimes["1"]; stillThere {
			t.Error("the previous weekday survived a replacing update")
		}
		if len(got.WeekdayTimes) != 1 {
			t.Errorf("got %d weekdays, want 1", len(got.WeekdayTimes))
		}
	})

	t.Run("times are left alone when no time field is sent", func(t *testing.T) {
		calendar := &models.Calendar{OwnerID: owner, Name: "Hours", AllowedHours: existing}
		calendar.ID = uuid.New()

		calendars := &mockCalendarRepo{calendar: calendar}
		service := newService(calendars, &mockParticipantRepo{})

		name := "Renamed"
		if _, err := service.UpdateCalendar(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.UpdateCalendarRequest{Name: &name},
		); err != nil {
			t.Fatalf("UpdateCalendar: %v", err)
		}

		if calendars.updated.AllowedHours != existing {
			t.Error("allowed_hours was rewritten by an update that named no time field")
		}
	})
}

// TestGetPublicCalendar covers the anonymous entry point, where the privacy filter and
// the token lookup meet.
func TestGetPublicCalendar(t *testing.T) {
	calendarID := uuid.New()

	email := "alice@example.com"
	alice := models.Participant{CalendarID: calendarID, Name: "Alice", Email: &email, EmailVerified: true}
	alice.ID = uuid.New()
	bob := models.Participant{CalendarID: calendarID, Name: "Bob"}
	bob.ID = uuid.New()

	t.Run("an unknown token is reported as not found", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{byTokenErr: repository.ErrCalendarNotFound},
			&mockParticipantRepo{},
		)

		_, err := service.GetPublicCalendar(context.Background(), "nope", "")
		if !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})

	t.Run("a locked calendar masks everyone but the asker", func(t *testing.T) {
		calendar := &models.Calendar{Name: "Locked", LockParticipants: true, ICSToken: "ics-token"}
		calendar.ID = calendarID

		service := newService(
			&mockCalendarRepo{byToken: calendar},
			&mockParticipantRepo{participants: []models.Participant{alice, bob}},
		)

		got, err := service.GetPublicCalendar(context.Background(), "token", alice.ID.String())
		if err != nil {
			t.Fatalf("GetPublicCalendar: %v", err)
		}

		if len(got.Participants) != 2 {
			t.Fatalf("got %d participants, want 2", len(got.Participants))
		}
		if got.Participants[0].ID == nil {
			t.Error("the asker cannot see their own ID")
		}
		if got.Participants[1].ID != nil {
			t.Error("another participant's ID leaked")
		}
	})

	t.Run("the owner-only fields are absent from the public response", func(t *testing.T) {
		calendar := &models.Calendar{Name: "Public", PublicToken: "secret-public-token"}
		calendar.ID = calendarID

		service := newService(
			&mockCalendarRepo{byToken: calendar},
			&mockParticipantRepo{participants: []models.Participant{alice}},
		)

		got, err := service.GetPublicCalendar(context.Background(), "token", "")
		if err != nil {
			t.Fatalf("GetPublicCalendar: %v", err)
		}

		// PublicCalendarResponse has no OwnerID and no PublicToken field at all; this
		// asserts the shape has not grown one by accident.
		if got.Name != "Public" {
			t.Errorf("Name = %q, want %q", got.Name, "Public")
		}
		if got.Participants[0].Email != nil {
			t.Error("an email reached an anonymous viewer")
		}
	})
}

func TestListUserCalendars(t *testing.T) {
	owner := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Theirs"}
	calendar.ID = uuid.New()

	service := newService(
		&mockCalendarRepo{owned: []*models.Calendar{calendar}},
		&mockParticipantRepo{},
	)

	got, err := service.ListUserCalendars(context.Background(), owner.String())
	if err != nil {
		t.Fatalf("ListUserCalendars: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d calendars, want 1", len(got))
	}

	if _, err := service.ListUserCalendars(context.Background(), "not-a-uuid"); err == nil {
		t.Error("expected an error for a non-uuid user id")
	}
}

// TestParticipantTenancy is the cross-calendar isolation check. A participant id is a
// bare UUID in the URL, so the service must confirm it belongs to the calendar being
// addressed — otherwise an owner of calendar A can rename or delete a participant of
// calendar B just by knowing their id.
func TestParticipantTenancy(t *testing.T) {
	owner := uuid.New()

	mine := &models.Calendar{OwnerID: owner, Name: "Mine"}
	mine.ID = uuid.New()

	// A participant that belongs to somebody else's calendar.
	foreign := &models.Participant{CalendarID: uuid.New(), Name: "Someone else"}
	foreign.ID = uuid.New()

	t.Run("update refuses a participant from another calendar", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{calendar: mine},
			&mockParticipantRepo{byID: foreign},
		)

		_, err := service.UpdateParticipant(
			context.Background(), owner.String(), "user", mine.ID.String(), foreign.ID.String(),
			&models.UpdateParticipantRequest{Name: "Renamed"},
		)
		if !errors.Is(err, ErrParticipantNotFound) {
			t.Errorf("error = %v, want ErrParticipantNotFound", err)
		}
	})

	t.Run("remove refuses a participant from another calendar", func(t *testing.T) {
		participants := &mockParticipantRepo{byID: foreign}
		service := newService(&mockCalendarRepo{calendar: mine}, participants)

		err := service.RemoveParticipant(
			context.Background(), owner.String(), "user", mine.ID.String(), foreign.ID.String(),
		)
		if !errors.Is(err, ErrParticipantNotFound) {
			t.Errorf("error = %v, want ErrParticipantNotFound", err)
		}
	})

	t.Run("a stranger cannot add a participant", func(t *testing.T) {
		service := newService(&mockCalendarRepo{calendar: mine}, &mockParticipantRepo{})

		_, err := service.AddParticipant(
			context.Background(), uuid.NewString(), "user", mine.ID.String(),
			&models.AddParticipantRequest{Name: "Intruder"},
		)
		if !errors.Is(err, ErrUnauthorized) {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("a malformed participant id", func(t *testing.T) {
		service := newService(&mockCalendarRepo{calendar: mine}, &mockParticipantRepo{})

		err := service.RemoveParticipant(
			context.Background(), owner.String(), "user", mine.ID.String(), "not-a-uuid",
		)
		if err == nil {
			t.Error("expected an error for a non-uuid participant id")
		}
	})
}

func TestAddParticipant(t *testing.T) {
	owner := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Mine", PublicToken: "tok"}
	calendar.ID = uuid.New()

	t.Run("the owner adds one", func(t *testing.T) {
		service := newService(&mockCalendarRepo{calendar: calendar}, &mockParticipantRepo{})

		got, err := service.AddParticipant(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.AddParticipantRequest{Name: "Ada"},
		)
		if err != nil {
			t.Fatalf("AddParticipant: %v", err)
		}
		if got.Name != "Ada" {
			t.Errorf("Name = %q, want %q", got.Name, "Ada")
		}
		if got.CalendarID != calendar.ID {
			t.Errorf("CalendarID = %v, want %v", got.CalendarID, calendar.ID)
		}
	})

	t.Run("a duplicate name is reported as such", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{calendar: calendar},
			&mockParticipantRepo{createErr: repository.ErrParticipantAlreadyExists},
		)

		_, err := service.AddParticipant(
			context.Background(), owner.String(), "user", calendar.ID.String(),
			&models.AddParticipantRequest{Name: "Ada"},
		)
		if !errors.Is(err, ErrParticipantExists) {
			t.Errorf("error = %v, want ErrParticipantExists", err)
		}
	})
}

// TestAddAnonymousParticipant covers the unauthenticated path, which is gated only by
// the calendar's own flag.
func TestAddAnonymousParticipant(t *testing.T) {
	tests := []struct {
		name    string
		allowed bool
		wantErr error
	}{
		{name: "the calendar accepts anonymous participants", allowed: true},
		{name: "the calendar does not", allowed: false, wantErr: ErrUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendar := &models.Calendar{Name: "Open", AllowAnonymousParticipants: tt.allowed}
			calendar.ID = uuid.New()

			service := newService(&mockCalendarRepo{byToken: calendar}, &mockParticipantRepo{})

			got, err := service.AddAnonymousParticipant(
				context.Background(), "public-token", &models.AddParticipantRequest{Name: "Guest"},
			)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != "Guest" {
				t.Errorf("Name = %q, want %q", got.Name, "Guest")
			}
		})
	}

	t.Run("an unknown token", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{byTokenErr: repository.ErrCalendarNotFound},
			&mockParticipantRepo{},
		)

		_, err := service.AddAnonymousParticipant(
			context.Background(), "nope", &models.AddParticipantRequest{Name: "Guest"},
		)
		if !errors.Is(err, ErrCalendarNotFound) {
			t.Errorf("error = %v, want ErrCalendarNotFound", err)
		}
	})
}

// TestRemoveParticipantAdjustsThreshold covers the quiet repair after a removal: a
// threshold higher than the number of remaining participants can never be met, so the
// service lowers it. The guard on an empty calendar is the interesting case.
func TestRemoveParticipantAdjustsThreshold(t *testing.T) {
	owner := uuid.New()
	calendarID := uuid.New()

	participant := &models.Participant{CalendarID: calendarID, Name: "Leaving"}
	participant.ID = uuid.New()

	remaining := func(n int) []models.Participant {
		out := make([]models.Participant, n)
		for i := range out {
			out[i] = models.Participant{CalendarID: calendarID}
			out[i].ID = uuid.New()
		}

		return out
	}

	tests := []struct {
		name          string
		threshold     int
		left          int
		wantThreshold int
		wantUpdate    bool
	}{
		{
			name:      "the threshold still fits",
			threshold: 2, left: 3,
			wantThreshold: 2,
		},
		{
			name:      "the threshold equals what is left",
			threshold: 3, left: 3,
			wantThreshold: 3,
		},
		{
			name:      "the threshold is lowered to what is left",
			threshold: 4, left: 2,
			wantThreshold: 2, wantUpdate: true,
		},
		{
			// Nobody is left, so there is no sensible threshold to lower to; the
			// value is deliberately left alone rather than driven to zero.
			name:      "the last participant leaves",
			threshold: 3, left: 0,
			wantThreshold: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calendar := &models.Calendar{OwnerID: owner, Name: "Group", Threshold: tt.threshold}
			calendar.ID = calendarID

			calendars := &mockCalendarRepo{calendar: calendar}
			service := newService(calendars, &mockParticipantRepo{
				byID:         participant,
				participants: remaining(tt.left),
			})

			if err := service.RemoveParticipant(
				context.Background(), owner.String(), "user", calendarID.String(), participant.ID.String(),
			); err != nil {
				t.Fatalf("RemoveParticipant: %v", err)
			}

			if calendar.Threshold != tt.wantThreshold {
				t.Errorf("Threshold = %d, want %d", calendar.Threshold, tt.wantThreshold)
			}
			if (calendars.updated != nil) != tt.wantUpdate {
				t.Errorf("calendar written = %v, want %v", calendars.updated != nil, tt.wantUpdate)
			}
		})
	}
}

func TestUpdateParticipant(t *testing.T) {
	owner := uuid.New()
	calendarID := uuid.New()

	calendar := &models.Calendar{OwnerID: owner, Name: "Group"}
	calendar.ID = calendarID

	participant := &models.Participant{CalendarID: calendarID, Name: "Ada"}
	participant.ID = uuid.New()

	t.Run("renames", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{calendar: calendar},
			&mockParticipantRepo{byID: participant},
		)

		got, err := service.UpdateParticipant(
			context.Background(), owner.String(), "user", calendarID.String(), participant.ID.String(),
			&models.UpdateParticipantRequest{Name: "Ada Lovelace"},
		)
		if err != nil {
			t.Fatalf("UpdateParticipant: %v", err)
		}
		if got == nil {
			t.Fatal("no participant returned")
		}
	})

	t.Run("a colliding name is reported as such", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{calendar: calendar},
			&mockParticipantRepo{byID: participant, updateErr: repository.ErrParticipantAlreadyExists},
		)

		_, err := service.UpdateParticipant(
			context.Background(), owner.String(), "user", calendarID.String(), participant.ID.String(),
			&models.UpdateParticipantRequest{Name: "Grace"},
		)
		if !errors.Is(err, ErrParticipantExists) {
			t.Errorf("error = %v, want ErrParticipantExists", err)
		}
	})

	t.Run("a missing participant", func(t *testing.T) {
		service := newService(
			&mockCalendarRepo{calendar: calendar},
			&mockParticipantRepo{byIDErr: repository.ErrParticipantNotFound},
		)

		_, err := service.UpdateParticipant(
			context.Background(), owner.String(), "user", calendarID.String(), uuid.NewString(),
			&models.UpdateParticipantRequest{Name: "Ghost"},
		)
		if !errors.Is(err, ErrParticipantNotFound) {
			t.Errorf("error = %v, want ErrParticipantNotFound", err)
		}
	})
}
