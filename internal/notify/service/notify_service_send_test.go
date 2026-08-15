// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/whento/pkg/email"
	authModels "github.com/whento/whento/internal/auth/models"
	availabilityModels "github.com/whento/whento/internal/availability/models"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// The whole notification path — who is told, who is told twice, who is told
// nothing — used to be unreachable from a test: the service held four concrete
// repositories, a *email.Service and two concrete collaborators, so exercising it
// meant a database and an SMTP server. It now takes interfaces, and these are the
// hand-written fakes that stand in for them.

type fakeCalendarStore struct {
	calendar *calendarModels.Calendar
	err      error
}

func (f *fakeCalendarStore) GetByID(_ context.Context, _ uuid.UUID) (*calendarModels.Calendar, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.calendar, nil
}

type fakeParticipantStore struct {
	all      []calendarModels.Participant
	verified []calendarModels.Participant
	allErr   error
	verErr   error
}

func (f *fakeParticipantStore) GetByCalendarID(_ context.Context, _ uuid.UUID) ([]calendarModels.Participant, error) {
	return f.all, f.allErr
}

func (f *fakeParticipantStore) GetVerifiedParticipantsByCalendar(_ context.Context, _ uuid.UUID) ([]calendarModels.Participant, error) {
	return f.verified, f.verErr
}

type fakeAvailabilityStore struct {
	available []availabilityModels.AvailableParticipant
	err       error
}

func (f *fakeAvailabilityStore) GetAvailableParticipantsForDate(
	_ context.Context, _ uuid.UUID, _ time.Time,
) ([]availabilityModels.AvailableParticipant, error) {
	return f.available, f.err
}

type fakeUserStore struct {
	user *authModels.User
	err  error
}

func (f *fakeUserStore) GetByID(_ context.Context, _ uuid.UUID) (*authModels.User, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.user, nil
}

// loggedNotification is one row of the anti-spam ledger.
type loggedNotification struct {
	recipientType string
	recipientID   uuid.UUID
	channel       string
}

type fakeNotificationLog struct {
	// sentRecently is keyed by "recipientID/channel"; anything absent is new.
	sentRecently map[string]bool
	checkErr     error
	logged       []loggedNotification
}

func (f *fakeNotificationLog) WasNotificationSentRecently(
	_ context.Context, _ uuid.UUID, _ time.Time, _ string, recipientID uuid.UUID, channel string,
) (bool, error) {
	if f.checkErr != nil {
		return false, f.checkErr
	}
	return f.sentRecently[recipientID.String()+"/"+channel], nil
}

func (f *fakeNotificationLog) LogNotification(
	_ context.Context, _ uuid.UUID, _ time.Time, _, recipientType string, recipientID uuid.UUID, channel string,
) error {
	f.logged = append(f.logged, loggedNotification{recipientType, recipientID, channel})
	return nil
}

// fakeMailer is guarded by a mutex because two of the senders under test hand the
// actual delivery to a goroutine and return before it runs.
type fakeMailer struct {
	mu         sync.Mutex
	configured bool
	err        error
	sent       []email.Email
}

func (f *fakeMailer) IsConfigured() bool { return f.configured }

func (f *fakeMailer) Send(msg email.Email) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, msg)
	return nil
}

// messages returns what has been sent so far.
func (f *fakeMailer) messages() []email.Email {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]email.Email(nil), f.sent...)
}

// awaitMessages waits for n messages to have been handed to the mailer, which is
// how a fire-and-forget send is observed without a sleep long enough to be slow and
// short enough to be flaky.
func (f *fakeMailer) awaitMessages(t *testing.T, n int) []email.Email {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		msgs := f.messages()
		if len(msgs) >= n {
			return msgs
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d message(s), got %d after 2s", n, len(msgs))
		}
		time.Sleep(time.Millisecond)
	}
}

type externalCall struct {
	channel string
	target  string
	message string
}

type fakeChannelNotifier struct {
	calls []externalCall
	err   error
}

func (f *fakeChannelNotifier) SendDiscord(_ context.Context, webhookURL, message string) error {
	f.calls = append(f.calls, externalCall{"discord", webhookURL, message})
	return f.err
}

func (f *fakeChannelNotifier) SendSlack(_ context.Context, webhookURL, message string) error {
	f.calls = append(f.calls, externalCall{"slack", webhookURL, message})
	return f.err
}

func (f *fakeChannelNotifier) SendTelegram(_ context.Context, botToken, chatID, message string) error {
	f.calls = append(f.calls, externalCall{"telegram", botToken + "/" + chatID, message})
	return f.err
}

type fakeDetector struct {
	transition *models.ThresholdTransition
	err        error
}

func (f *fakeDetector) DetectTransition(
	_ context.Context, calendarID uuid.UUID, date time.Time, threshold, previousCount int,
) (*models.ThresholdTransition, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.transition != nil {
		return f.transition, nil
	}
	return &models.ThresholdTransition{
		CalendarID:     calendarID,
		Date:           date,
		PreviousCount:  previousCount,
		NewCount:       threshold,
		Threshold:      threshold,
		TransitionType: "threshold_reached",
	}, nil
}

// Compile-time proof that the fakes still match what the service asks for.
var (
	_ CalendarStore      = (*fakeCalendarStore)(nil)
	_ ParticipantStore   = (*fakeParticipantStore)(nil)
	_ AvailabilityStore  = (*fakeAvailabilityStore)(nil)
	_ UserStore          = (*fakeUserStore)(nil)
	_ NotificationLog    = (*fakeNotificationLog)(nil)
	_ Mailer             = (*fakeMailer)(nil)
	_ ChannelNotifier    = (*fakeChannelNotifier)(nil)
	_ TransitionDetector = (*fakeDetector)(nil)
)

func strptr(s string) *string { return &s }

// notifyFixture is one calendar, one owner, one participant and the eight
// collaborators, assembled so a test only has to change what it is about.
type notifyFixture struct {
	calendar    *calendarModels.Calendar
	owner       *authModels.User
	participant calendarModels.Participant
	calendars   *fakeCalendarStore
	people      *fakeParticipantStore
	slots       *fakeAvailabilityStore
	users       *fakeUserStore
	log         *fakeNotificationLog
	mailer      *fakeMailer
	external    *fakeChannelNotifier
	detector    *fakeDetector
	date        time.Time
}

func newNotifyFixture(t *testing.T, config models.NotifyConfig) *notifyFixture {
	t.Helper()

	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal notify config: %v", err)
	}
	raw := string(configJSON)

	calendarID := uuid.New()
	ownerID := uuid.New()
	date := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)

	calendar := &calendarModels.Calendar{
		OwnerID:           ownerID,
		Name:              "Board game night",
		PublicToken:       "public-token",
		Threshold:         2,
		NotifyOnThreshold: true,
		NotifyConfig:      &raw,
	}
	calendar.ID = calendarID

	owner := &authModels.User{
		Email:         "owner@example.test",
		DisplayName:   "Owner",
		Locale:        "en",
		EmailVerified: true,
	}
	owner.ID = ownerID

	participant := calendarModels.Participant{
		CalendarID:    calendarID,
		Name:          "Ada",
		Email:         strptr("ada@example.test"),
		EmailVerified: true,
		Locale:        "en",
	}
	participant.ID = uuid.New()

	ownerParticipant := calendarModels.Participant{
		CalendarID:    calendarID,
		Name:          "Owner",
		Email:         strptr(owner.Email),
		EmailVerified: true,
		Locale:        "en",
	}
	ownerParticipant.ID = uuid.New()

	// The store answers with participants now, not rows: whether an availability was
	// typed in or comes from a recurrence is the repository's business, and the
	// distinction had no place in this service.
	available := []availabilityModels.AvailableParticipant{
		{ID: participant.ID, Name: participant.Name},
		{ID: ownerParticipant.ID, Name: ownerParticipant.Name},
	}

	return &notifyFixture{
		calendar:    calendar,
		owner:       owner,
		participant: participant,
		calendars:   &fakeCalendarStore{calendar: calendar},
		people: &fakeParticipantStore{
			all:      []calendarModels.Participant{participant, ownerParticipant},
			verified: []calendarModels.Participant{participant, ownerParticipant},
		},
		slots:    &fakeAvailabilityStore{available: available},
		users:    &fakeUserStore{user: owner},
		log:      &fakeNotificationLog{sentRecently: map[string]bool{}},
		mailer:   &fakeMailer{configured: true},
		external: &fakeChannelNotifier{},
		detector: &fakeDetector{},
		date:     date,
	}
}

func (f *notifyFixture) service() *NotifyService {
	return NewNotifyService(
		f.calendars, f.people, f.slots, f.users, f.log,
		f.mailer, f.external, f.detector,
		"https://whento.test", quietLogger(),
	)
}

func emailConfig() models.NotifyConfig {
	return models.NotifyConfig{
		Enabled:            true,
		NotifyOwner:        true,
		NotifyParticipants: true,
		Channels:           models.ChannelConfig{Email: models.EmailChannelConfig{Enabled: true}},
	}
}

// TestCheckThresholdAndNotifyGates covers every reason the service declines to
// notify at all. Each of them used to be a branch nothing could reach.
func TestCheckThresholdAndNotifyGates(t *testing.T) {
	tests := []struct {
		name    string
		arrange func(*notifyFixture)
		wantErr bool
	}{
		{
			name:    "calendar lookup fails",
			arrange: func(f *notifyFixture) { f.calendars.err = errors.New("database down") },
			wantErr: true,
		},
		{
			name:    "notifications switched off on the calendar",
			arrange: func(f *notifyFixture) { f.calendar.NotifyOnThreshold = false },
		},
		{
			name:    "no notify config stored",
			arrange: func(f *notifyFixture) { f.calendar.NotifyConfig = nil },
		},
		{
			name: "notify config is not valid JSON",
			arrange: func(f *notifyFixture) {
				broken := "{not json"
				f.calendar.NotifyConfig = &broken
			},
			wantErr: true,
		},
		{
			name: "config disabled",
			arrange: func(f *notifyFixture) {
				disabled := `{"enabled":false,"notify_owner":true}`
				f.calendar.NotifyConfig = &disabled
			},
		},
		{
			name:    "the detector fails",
			arrange: func(f *notifyFixture) { f.detector.err = errors.New("count failed") },
			wantErr: true,
		},
		{
			name: "no transition to report",
			arrange: func(f *notifyFixture) {
				f.detector.transition = &models.ThresholdTransition{TransitionType: "none"}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNotifyFixture(t, emailConfig())
			tt.arrange(f)

			err := f.service().CheckThresholdAndNotify(context.Background(), f.calendar.ID, f.date, 1)

			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(f.mailer.messages()) != 0 {
				t.Errorf("mail was sent despite the gate: %d message(s)", len(f.mailer.messages()))
			}
			if len(f.external.calls) != 0 {
				t.Errorf("an external channel was called despite the gate: %v", f.external.calls)
			}
		})
	}
}

// TestCheckThresholdAndNotifyDeduplicatesRecipients is the point of the whole
// email path: the owner is also a participant here, under the same address, and
// must be mailed once, as the owner, with the personalised link.
func TestCheckThresholdAndNotifyDeduplicatesRecipients(t *testing.T) {
	f := newNotifyFixture(t, emailConfig())

	if err := f.service().CheckThresholdAndNotify(context.Background(), f.calendar.ID, f.date, 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f.mailer.messages()) != 2 {
		t.Fatalf("expected one message per unique address (owner + Ada), got %d", len(f.mailer.messages()))
	}

	seen := map[string]int{}
	for _, msg := range f.mailer.messages() {
		if len(msg.To) != 1 {
			t.Fatalf("expected a single recipient per message, got %v", msg.To)
		}
		seen[msg.To[0]]++
		if !msg.HTML {
			t.Error("threshold notifications are sent as HTML")
		}
	}
	if seen["owner@example.test"] != 1 {
		t.Errorf("the owner should be mailed exactly once, got %d", seen["owner@example.test"])
	}
	if seen["ada@example.test"] != 1 {
		t.Errorf("Ada should be mailed exactly once, got %d", seen["ada@example.test"])
	}

	var ownerRows, participantRows int
	for _, row := range f.log.logged {
		if row.channel != "email" {
			continue
		}
		if row.recipientType == "owner" {
			ownerRows++
		} else {
			participantRows++
		}
	}
	if ownerRows != 1 || participantRows != 1 {
		t.Errorf("ledger should hold one owner row and one participant row, got %d and %d", ownerRows, participantRows)
	}
}

// TestCheckThresholdAndNotifyEmailRecipientRules covers who is left out.
func TestCheckThresholdAndNotifyEmailRecipientRules(t *testing.T) {
	tests := []struct {
		name    string
		config  models.NotifyConfig
		arrange func(*notifyFixture)
		wantTo  []string
		// wantBody is checked against every message sent, for the cases where who is
		// listed in the mail matters as much as who receives it.
		wantBody   []string
		wantNoMail bool
	}{
		{
			name:       "email channel disabled",
			config:     models.NotifyConfig{Enabled: true, NotifyOwner: true},
			wantNoMail: true,
		},
		{
			name:       "SMTP not configured",
			config:     emailConfig(),
			arrange:    func(f *notifyFixture) { f.mailer.configured = false },
			wantNoMail: true,
		},
		{
			name: "owner only",
			config: models.NotifyConfig{
				Enabled:     true,
				NotifyOwner: true,
				Channels:    models.ChannelConfig{Email: models.EmailChannelConfig{Enabled: true}},
			},
			wantTo: []string{"owner@example.test"},
		},
		{
			name: "participants only, and only those free on the date",
			config: models.NotifyConfig{
				Enabled:            true,
				NotifyParticipants: true,
				Channels:           models.ChannelConfig{Email: models.EmailChannelConfig{Enabled: true}},
			},
			arrange: func(f *notifyFixture) {
				// Only Ada declared herself available, so the owner participant
				// is not told even though her address is verified.
				f.slots.available = []availabilityModels.AvailableParticipant{
					{ID: f.participant.ID, Name: f.participant.Name},
				}
			},
			wantTo: []string{"ada@example.test"},
		},
		{
			// The reported defect. Recurrences are expanded when read and never
			// stored, so a service asking the availabilities table for rows saw
			// nobody — while the threshold count, which expands them, saw three of
			// three. The participant was counted towards the event, left out of the
			// list in the email, and never told about their own Friday.
			name: "a participant available only through a recurrence is told, and listed",
			config: models.NotifyConfig{
				Enabled:            true,
				NotifyParticipants: true,
				Channels:           models.ChannelConfig{Email: models.EmailChannelConfig{Enabled: true}},
			},
			arrange: func(f *notifyFixture) {
				// The store answers with the participant either way; what used to
				// differ is whether a row existed to answer from.
				f.slots.available = []availabilityModels.AvailableParticipant{
					{ID: f.participant.ID, Name: f.participant.Name},
				}
			},
			wantTo:   []string{"ada@example.test"},
			wantBody: []string{"Ada"},
		},
		{
			name:   "nobody is available on the date",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				f.slots.available = nil
			},
			wantTo: []string{"owner@example.test"},
		},
		{
			name:   "a participant without a verified address is skipped",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				unverified := f.participant
				unverified.EmailVerified = false
				f.people.verified = []calendarModels.Participant{unverified}
			},
			wantTo: []string{"owner@example.test"},
		},
		{
			name:   "the same notice was already sent recently",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				f.log.sentRecently = map[string]bool{
					f.owner.ID.String() + "/email":       true,
					f.participant.ID.String() + "/email": true,
				}
			},
			wantNoMail: true,
		},
		{
			name:   "the ledger cannot be read, so the notice goes out anyway",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				f.log.checkErr = errors.New("ledger unavailable")
				f.people.verified = nil
			},
			wantTo: []string{"owner@example.test"},
		},
		{
			name:   "the owner cannot be resolved",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				f.users.err = errors.New("no such user")
				f.slots.available = []availabilityModels.AvailableParticipant{
					{ID: f.participant.ID, Name: f.participant.Name},
				}
				f.people.verified = []calendarModels.Participant{f.participant}
			},
			wantTo: []string{"ada@example.test"},
		},
		{
			name:   "sending fails, so nothing is written to the ledger",
			config: emailConfig(),
			arrange: func(f *notifyFixture) {
				f.mailer.err = errors.New("smtp refused")
			},
			wantNoMail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNotifyFixture(t, tt.config)
			if tt.arrange != nil {
				tt.arrange(f)
			}

			if err := f.service().CheckThresholdAndNotify(context.Background(), f.calendar.ID, f.date, 1); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNoMail {
				if len(f.mailer.messages()) != 0 {
					t.Fatalf("expected no mail, got %d message(s)", len(f.mailer.messages()))
				}
				for _, row := range f.log.logged {
					if row.channel == "email" {
						t.Errorf("an unsent email was recorded in the ledger: %+v", row)
					}
				}
				return
			}

			got := make([]string, 0, len(f.mailer.messages()))
			for _, msg := range f.mailer.messages() {
				got = append(got, msg.To...)
			}
			if len(got) != len(tt.wantTo) {
				t.Fatalf("expected %v, got %v", tt.wantTo, got)
			}
			for _, want := range tt.wantTo {
				found := false
				for _, addr := range got {
					if addr == want {
						found = true
					}
				}
				if !found {
					t.Errorf("expected %s among the recipients, got %v", want, got)
				}
			}

			for _, want := range tt.wantBody {
				for _, msg := range f.mailer.messages() {
					if !strings.Contains(msg.Body, want) {
						t.Errorf("the mail body is missing %q:\n%s", want, msg.Body)
					}
				}
			}
		})
	}
}

// TestCheckThresholdAndNotifyExternalChannels covers the owner-only chat channels,
// including the credentials that must be present before anything is posted.
func TestCheckThresholdAndNotifyExternalChannels(t *testing.T) {
	full := func() models.NotifyConfig {
		return models.NotifyConfig{
			Enabled:     true,
			NotifyOwner: true,
			Channels: models.ChannelConfig{
				Discord:  models.DiscordChannelConfig{Enabled: true, WebhookURL: "https://discord.test/webhooks/1/abcdefghijklmnop"},
				Slack:    models.SlackChannelConfig{Enabled: true, WebhookURL: "https://hooks.slack.test/services/abcdefghijklmnop"},
				Telegram: models.TelegramChannelConfig{Enabled: true, BotToken: "123:abc", ChatID: "chat-1"},
			},
		}
	}

	tests := []struct {
		name        string
		config      models.NotifyConfig
		arrange     func(*notifyFixture)
		wantChannel []string
	}{
		{
			name:        "all three channels configured",
			config:      full(),
			wantChannel: []string{"discord", "slack", "telegram"},
		},
		{
			name: "channels enabled but without credentials post nothing",
			config: models.NotifyConfig{
				Enabled:     true,
				NotifyOwner: true,
				Channels: models.ChannelConfig{
					Discord:  models.DiscordChannelConfig{Enabled: true},
					Slack:    models.SlackChannelConfig{Enabled: true},
					Telegram: models.TelegramChannelConfig{Enabled: true, BotToken: "123:abc"},
				},
			},
		},
		{
			name:   "already posted recently",
			config: full(),
			arrange: func(f *notifyFixture) {
				f.log.sentRecently = map[string]bool{
					f.owner.ID.String() + "/discord":  true,
					f.owner.ID.String() + "/slack":    true,
					f.owner.ID.String() + "/telegram": true,
				}
			},
		},
		{
			name:        "a failing channel is not recorded as sent",
			config:      full(),
			arrange:     func(f *notifyFixture) { f.external.err = errors.New("webhook rejected") },
			wantChannel: []string{"discord", "slack", "telegram"},
		},
		{
			name:    "the owner cannot be resolved, so nothing is posted",
			config:  full(),
			arrange: func(f *notifyFixture) { f.users.err = errors.New("no such user") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newNotifyFixture(t, tt.config)
			if tt.arrange != nil {
				tt.arrange(f)
			}

			if err := f.service().CheckThresholdAndNotify(context.Background(), f.calendar.ID, f.date, 1); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := make([]string, 0, len(f.external.calls))
			for _, call := range f.external.calls {
				got = append(got, call.channel)
				if !strings.Contains(call.message, "Board game night") {
					t.Errorf("the %s message does not name the calendar: %q", call.channel, call.message)
				}
			}
			if len(got) != len(tt.wantChannel) {
				t.Fatalf("expected calls to %v, got %v", tt.wantChannel, got)
			}

			// A channel that refused must leave no trace in the ledger, or the
			// retry an hour later would be suppressed as a duplicate.
			if f.external.err != nil {
				for _, row := range f.log.logged {
					if row.channel != "email" {
						t.Errorf("a failed %s post was recorded as sent", row.channel)
					}
				}
			}
		})
	}
}

// TestBuildHTMLNotificationMessage checks the parts of the mail that carry meaning:
// the locale, the escaping of names, and the cancel link that only a recipient with
// a participant identity can be offered.
func TestBuildHTMLNotificationMessage(t *testing.T) {
	calendar := &calendarModels.Calendar{Name: `Ada & <b>Bob</b>`}
	transition := &models.ThresholdTransition{
		Date:           time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
		NewCount:       2,
		Threshold:      2,
		TransitionType: "threshold_reached",
	}
	service := &NotifyService{}

	tests := []struct {
		name             string
		locale           string
		transitionType   string
		hasParticipantID bool
		names            []string
		wantContains     []string
		wantAbsent       []string
	}{
		{
			name:             "english, with a cancel link and a participant list",
			locale:           "en",
			transitionType:   "threshold_reached",
			hasParticipantID: true,
			names:            []string{`Ada <script>`},
			wantContains: []string{
				"Threshold reached", "View Calendar", "Cancel my participation",
				"?cancel=2026-04-02", "Ada &lt;script&gt;", "Ada &amp; &lt;b&gt;Bob&lt;/b&gt;",
			},
		},
		{
			name:           "english, no participant identity means no cancel link",
			locale:         "en",
			transitionType: "threshold_reached",
			wantContains:   []string{"View Calendar"},
			wantAbsent:     []string{"Cancel my participation", "?cancel="},
		},
		{
			name:           "french",
			locale:         "fr",
			transitionType: "threshold_reached",
			wantContains:   []string{"Seuil atteint", "Voir le calendrier"},
			wantAbsent:     []string{"Threshold reached"},
		},
		{
			name:           "french, threshold lost",
			locale:         "fr",
			transitionType: "threshold_lost",
			wantContains:   []string{"Seuil perdu"},
		},
		{
			name:           "french, neither reached nor lost",
			locale:         "fr",
			transitionType: "none",
			wantContains:   []string{"Disponibilité modifiée"},
		},
		{
			name:           "english, threshold lost",
			locale:         "en",
			transitionType: "threshold_lost",
			wantContains:   []string{"Threshold lost"},
		},
		{
			name:           "an unknown locale falls back to english",
			locale:         "de",
			transitionType: "none",
			wantContains:   []string{"Availability changed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := *transition
			local.TransitionType = tt.transitionType

			got := service.buildHTMLNotificationMessage(
				calendar, &local, "https://whento.test/c/tok/p/pid", tt.hasParticipantID, tt.locale, tt.names,
			)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("message does not contain %q", want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("message unexpectedly contains %q", absent)
				}
			}
			if strings.Contains(got, "<script>") {
				t.Error("an unescaped <script> tag reached the mail body")
			}
		})
	}
}

// TestSendEmailNotificationSubject records that the subject line is localised and
// that the body is passed through untouched.
func TestSendEmailNotificationSubject(t *testing.T) {
	tests := []struct {
		locale      string
		wantSubject string
	}{
		{locale: "en", wantSubject: "WhenTo Calendar Notification"},
		{locale: "fr", wantSubject: "Notification de Calendrier WhenTo"},
		{locale: "", wantSubject: "WhenTo Calendar Notification"},
	}

	for _, tt := range tests {
		t.Run("locale "+tt.locale, func(t *testing.T) {
			mailer := &fakeMailer{configured: true}
			service := &NotifyService{emailService: mailer, logger: quietLogger()}

			err := service.sendEmailNotification("someone@example.test", "<p>body</p>", tt.locale, true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(mailer.messages()) != 1 {
				t.Fatalf("expected one message, got %d", len(mailer.messages()))
			}
			if mailer.messages()[0].Subject != tt.wantSubject {
				t.Errorf("subject = %q, want %q", mailer.messages()[0].Subject, tt.wantSubject)
			}
			if mailer.messages()[0].Body != "<p>body</p>" {
				t.Errorf("body was altered: %q", mailer.messages()[0].Body)
			}
		})
	}
}

// TestSendEmailNotificationPropagatesFailure keeps the caller's ability to tell a
// refused send from a delivered one — the ledger depends on it.
func TestSendEmailNotificationPropagatesFailure(t *testing.T) {
	mailer := &fakeMailer{configured: true, err: errors.New("smtp refused")}
	service := &NotifyService{emailService: mailer, logger: quietLogger()}

	err := service.sendEmailNotification("a@example.test", "body", "en", false)
	if err == nil {
		t.Fatal("expected the SMTP failure to propagate")
	}
}
