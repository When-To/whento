// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	availabilityModels "github.com/whento/whento/internal/availability/models"
	calendarModels "github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/notify/models"
)

// A threshold notification used to say how many people were available and never who.
// These pin the two halves of the fix: the named message when the journal has an entry,
// and — the part worth guarding — the byte-identical old message when it has none.
//
// "Byte-identical" is the contract. An absent entry means the date predates the journal
// or the participant has been deleted, and neither is something to announce: no
// "(unknown)", no dangling dash, no trailing space.

func messageCalendar() *calendarModels.Calendar {
	calendar := &calendarModels.Calendar{Name: "Squash"}
	calendar.ID = uuid.New()

	return calendar
}

func transitionOf(kind string, joined, withdrawn *availabilityModels.ParticipantRef) *models.ThresholdTransition {
	date, _ := time.Parse("2006-01-02", "2026-03-05")

	return &models.ThresholdTransition{
		Date:           date,
		PreviousCount:  2,
		NewCount:       3,
		Threshold:      3,
		TransitionType: kind,
		LastJoined:     joined,
		LastWithdrawn:  withdrawn,
	}
}

func refTo(name string) *availabilityModels.ParticipantRef {
	return &availabilityModels.ParticipantRef{ID: uuid.New(), Name: name, At: time.Now()}
}

// TestBuildNotificationMessageNamesThePerson covers the text form, which is what the
// Discord, Slack and Telegram channels send. Those are owner-only, so the name is going
// to the one recipient who can already read every name on the calendar.
func TestBuildNotificationMessageNamesThePerson(t *testing.T) {
	tests := []struct {
		name       string
		transition *models.ThresholdTransition
		wantSubstr string
		wantAbsent string
	}{
		{
			name:       "threshold_reached names the last to join",
			transition: transitionOf("threshold_reached", refTo("Ada"), refTo("Grace")),
			wantSubstr: "last to join: Ada",
			// The withdrawal belongs to the other message. Naming both would say
			// something the transition does not mean.
			wantAbsent: "Grace",
		},
		{
			name:       "threshold_lost names whoever withdrew",
			transition: transitionOf("threshold_lost", refTo("Ada"), refTo("Grace")),
			wantSubstr: "withdrew: Grace",
			wantAbsent: "Ada",
		},
	}

	service := &NotifyService{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.buildNotificationMessage(messageCalendar(), tt.transition)

			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("message = %q, want it to contain %q", got, tt.wantSubstr)
			}
			if tt.wantAbsent != "" && strings.Contains(got, tt.wantAbsent) {
				t.Errorf("message = %q, want it NOT to contain %q", got, tt.wantAbsent)
			}
		})
	}
}

func TestBuildNotificationMessageWithoutAnEntryIsUnchanged(t *testing.T) {
	service := &NotifyService{}
	calendar := messageCalendar()

	tests := []struct {
		kind string
		want string
	}{
		{
			kind: "threshold_reached",
			want: "🎉 Calendar 'Squash': Threshold reached for 2026-03-05! (3/3 participants available)",
		},
		{
			kind: "threshold_lost",
			want: "⚠️ Calendar 'Squash': Threshold lost for 2026-03-05 (3/3 participants)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := service.buildNotificationMessage(calendar, transitionOf(tt.kind, nil, nil))

			// Equality, not Contains: a trailing separator left behind by an absent
			// name is exactly the sort of thing Contains would wave through.
			if got != tt.want {
				t.Errorf("message = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildHTMLNotificationMessageServesBothLocales covers the email body. The labels
// are hard-coded in Go rather than loaded from a locale file — only
// templates/locales/participant_email_verification.json works the other way — so both
// branches have to be exercised here or one of them silently rots.
func TestBuildHTMLNotificationMessageServesBothLocales(t *testing.T) {
	tests := []struct {
		name       string
		locale     string
		kind       string
		joined     *availabilityModels.ParticipantRef
		withdrawn  *availabilityModels.ParticipantRef
		wantSubstr string
	}{
		{
			name:       "en, reached",
			locale:     "en",
			kind:       "threshold_reached",
			joined:     refTo("Ada"),
			wantSubstr: "last to join: Ada",
		},
		{
			name:       "en, lost",
			locale:     "en",
			kind:       "threshold_lost",
			withdrawn:  refTo("Grace"),
			wantSubstr: "withdrew: Grace",
		},
		{
			name:       "fr, reached",
			locale:     "fr",
			kind:       "threshold_reached",
			joined:     refTo("Ada"),
			wantSubstr: "dernier inscrit : Ada",
		},
		{
			name:       "fr, lost",
			locale:     "fr",
			kind:       "threshold_lost",
			withdrawn:  refTo("Grace"),
			wantSubstr: "s'est retiré : Grace",
		},
	}

	service := &NotifyService{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.buildHTMLNotificationMessage(
				messageCalendar(),
				transitionOf(tt.kind, tt.joined, tt.withdrawn),
				"https://whento.test/c/token",
				false,
				tt.locale,
				nil,
			)

			if !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("body does not contain %q:\n%s", tt.wantSubstr, got)
			}
		})
	}
}

func TestBuildHTMLNotificationMessageEscapesTheName(t *testing.T) {
	service := &NotifyService{}

	got := service.buildHTMLNotificationMessage(
		messageCalendar(),
		transitionOf("threshold_reached", refTo(`<script>alert(1)</script>`), nil),
		"https://whento.test/c/token",
		false,
		"en",
		nil,
	)

	// A participant name is whatever somebody typed into a public form. It lands in an
	// HTML body, so it is escaped where it is interpolated — messageText itself is not
	// escaped downstream.
	if strings.Contains(got, "<script>") {
		t.Errorf("an unescaped participant name reached the HTML body:\n%s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Errorf("the escaped name is missing from the body:\n%s", got)
	}
}

func TestBuildHTMLNotificationMessageWithoutAnEntry(t *testing.T) {
	service := &NotifyService{}

	for _, locale := range []string{"en", "fr"} {
		t.Run(locale, func(t *testing.T) {
			got := service.buildHTMLNotificationMessage(
				messageCalendar(),
				transitionOf("threshold_reached", nil, nil),
				"https://whento.test/c/token",
				false,
				locale,
				nil,
			)

			// No name and no separator: with no entry the sentence keeps the shape it
			// had before the journal existed.
			if strings.Contains(got, "—") {
				t.Errorf("an empty entry left a separator behind:\n%s", got)
			}
		})
	}
}

// TestBuildHTMLNotificationMessageGolden pins the whole body, byte for byte, for every
// locale and transition. The substring tests above say what is in it; this one says
// nothing else moved. Regenerate the digests only for an intended change to the mail.
func TestBuildHTMLNotificationMessageGolden(t *testing.T) {
	want := map[string]string{
		"en/threshold_reached/false":    "418bf55c3b385ea507be3088899699ac1f74a37deb9e8da3202cece2a7f1ab5c",
		"en/threshold_reached/true":     "84477a138c381f5df9ccd3f731ca666ca32daa329a9a048ca0da7655734cbc17",
		"en/threshold_lost/false":       "9732214964b805d2bd39a4184656ee70289b883db152edacc5543e0851215dad",
		"en/threshold_lost/true":        "b456301b829df7c0a5001ae91e6be58d86b93f1172cdd00046a004ff266f32e7",
		"en/availability_changed/false": "5c522df2aa1734a5d2765ff510ac3f4aee5aaf6503cf28e3ef92eaec92062b5f",
		"en/availability_changed/true":  "027a5989edf0418efdbe232ce1f5a163ab9a0e9b3e70603869cc7341350a5231",
		"fr/threshold_reached/false":    "14190c823059d35dd5158885cb7e5f04fb16157c516fe00b13d5222c3a9e23a5",
		"fr/threshold_reached/true":     "d882543803eb2ea9ee205c506cde7a66bdddd4d99f7d036235680c6ca19ebbe0",
		"fr/threshold_lost/false":       "24d8d79c04724a40c9808f10bca55d04c11aa1d8ae78181fdee6d8d13aab2801",
		"fr/threshold_lost/true":        "ef84d20b2f4850a1876fecb32ba2cedc91e129a4efa7edd1e615492dd2c8f0d5",
		"fr/availability_changed/false": "1512d58f1bd2d64ec0dab5d358cf3627fb79e3b48d8ba0ec4e6765436b8c186a",
		"fr/availability_changed/true":  "b23427b42d6dde6b65ebc6b1ebbd5bde4e2c52350d55583b76cd1a86fecbbb78",
		"de/threshold_reached/false":    "418bf55c3b385ea507be3088899699ac1f74a37deb9e8da3202cece2a7f1ab5c",
		"de/threshold_reached/true":     "84477a138c381f5df9ccd3f731ca666ca32daa329a9a048ca0da7655734cbc17",
		"de/threshold_lost/false":       "9732214964b805d2bd39a4184656ee70289b883db152edacc5543e0851215dad",
		"de/threshold_lost/true":        "b456301b829df7c0a5001ae91e6be58d86b93f1172cdd00046a004ff266f32e7",
		"de/availability_changed/false": "5c522df2aa1734a5d2765ff510ac3f4aee5aaf6503cf28e3ef92eaec92062b5f",
		"de/availability_changed/true":  "027a5989edf0418efdbe232ce1f5a163ab9a0e9b3e70603869cc7341350a5231",
	}

	service := &NotifyService{}
	for _, locale := range []string{"en", "fr", "de"} {
		for _, kind := range []string{"threshold_reached", "threshold_lost", "availability_changed"} {
			for _, withParticipant := range []bool{false, true} {
				key := fmt.Sprintf("%s/%s/%t", locale, kind, withParticipant)
				got := service.buildHTMLNotificationMessage(
					messageCalendar(),
					transitionOf(kind, refTo("Ada"), refTo("Grace & <Co>")),
					"https://whento.test/c/token/p/pid",
					withParticipant,
					locale,
					[]string{"Ada", "Grace & <Co>"},
				)
				sum := sha256.Sum256([]byte(got))
				if digest := hex.EncodeToString(sum[:]); digest != want[key] {
					t.Errorf("%s: body digest %s, want %s", key, digest, want[key])
				}
			}
		}
	}
}
