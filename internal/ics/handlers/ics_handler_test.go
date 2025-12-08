// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// SPDX-License-Identifier: BSL-1.1

package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/whento/whento/internal/ics/handlers"
	"github.com/whento/whento/internal/ics/repository"
	"github.com/whento/whento/internal/ics/service"
)

// Mock repositories for testing (implement service interfaces)
type mockCalendarRepository struct {
	calendar *repository.Calendar
	err      error
}

// Ensure mockCalendarRepository implements service.CalendarRepository
var _ service.CalendarRepository = (*mockCalendarRepository)(nil)

func (m *mockCalendarRepository) GetByICSToken(ctx context.Context, icsToken string) (*repository.Calendar, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.calendar, nil
}

type mockAvailabilityRepository struct {
	events map[time.Time][]repository.DateAvailability
	err    error
}

// Ensure mockAvailabilityRepository implements service.AvailabilityRepository
var _ service.AvailabilityRepository = (*mockAvailabilityRepository)(nil)

func (m *mockAvailabilityRepository) GetEventsAboveThreshold(ctx context.Context, calendarID uuid.UUID, threshold int) (map[time.Time][]repository.DateAvailability, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.events, nil
}

type mockQuotaChecker struct {
	isOverQuota bool
	err         error
}

// Ensure mockQuotaChecker implements service.QuotaChecker
var _ service.QuotaChecker = (*mockQuotaChecker)(nil)

func (m *mockQuotaChecker) IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.isOverQuota, nil
}

func TestGetFeed_Success(t *testing.T) {
	// Setup mock data
	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Test Calendar",
			Threshold:         2,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6}, // Allow all days
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 3,
		},
	}

	date := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	startTime := "19:00"
	endTime := "23:00"

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:              date,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					Note:              "",
					AvailableCount:    2,
					TotalParticipants: 3,
				},
				{
					Date:              date,
					ParticipantName:   "Bob",
					StartTime:         &startTime,
					EndTime:           &endTime,
					Note:              "",
					AvailableCount:    2,
					TotalParticipants: 3,
				},
			},
		},
	}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/test-token.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "text/calendar; charset=utf-8" {
		t.Errorf("Expected Content-Type 'text/calendar; charset=utf-8', got '%s'", contentType)
	}

	// Check cache headers
	cacheControl := w.Header().Get("Cache-Control")
	if cacheControl != "no-cache, no-store, must-revalidate" {
		t.Errorf("Expected Cache-Control header with no-cache, got '%s'", cacheControl)
	}

	// Check body contains iCalendar data
	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty response body")
	}

	// Check for iCalendar markers
	if !strings.Contains(body, "BEGIN:VCALENDAR") {
		t.Error("Expected iCalendar to contain BEGIN:VCALENDAR")
	}
	if !strings.Contains(body, "END:VCALENDAR") {
		t.Error("Expected iCalendar to contain END:VCALENDAR")
	}
	if !strings.Contains(body, "Test Calendar #1") {
		t.Error("Expected event summary to contain 'Test Calendar #1'")
	}
}

func TestGetFeed_CalendarNotFound(t *testing.T) {
	// Setup mock with error
	mockCalRepo := &mockCalendarRepository{
		err: errors.New("calendar not found"),
	}
	mockAvailRepo := &mockAvailabilityRepository{}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/invalid-token", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "invalid-token")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status code 404, got %d", w.Code)
	}
}

func TestGetFeed_MissingToken(t *testing.T) {
	// Setup mocks
	mockCalRepo := &mockCalendarRepository{}
	mockAvailRepo := &mockAvailabilityRepository{}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request with empty token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code 400, got %d", w.Code)
	}
}

func TestGetFeed_UsesCorrectDomain(t *testing.T) {
	// Setup mock data
	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Test Calendar",
			Threshold:         1,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6}, // Allow all days
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 1,
		},
	}

	date := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	startTime := "10:00"
	endTime := "12:00"

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:              date,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    1,
					TotalParticipants: 1,
				},
			},
		},
	}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "default.example.com")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request with specific host
	req := httptest.NewRequest(http.MethodGet, "http://192.168.1.10:8080/api/v1/ics/feed/test-token.ics", nil)
	// Note: httptest.NewRequest automatically sets req.Host from the URL
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	// Check body contains the correct domain in UID
	body := w.Body.String()
	expectedUIDPart := fmt.Sprintf("@192.168.1.10:8080")
	if !strings.Contains(body, expectedUIDPart) {
		t.Errorf("Expected UID to contain '%s', but got:\n%s", expectedUIDPart, body)
	}

	// Make sure it doesn't contain the default domain
	if strings.Contains(body, "@default.example.com") {
		t.Error("UID should not contain the default domain when request host is provided")
	}
}

func TestGetFeed_UsesXForwardedHost(t *testing.T) {
	// Setup mock data
	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Test Calendar",
			Threshold:         1,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6}, // Allow all days
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 1,
		},
	}

	date := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	startTime := "10:00"
	endTime := "12:00"

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:              date,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    1,
					TotalParticipants: 1,
				},
			},
		},
	}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "default.example.com")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request - the Host will be set to localhost:5173 (backend)
	// but X-Forwarded-Host should contain the original frontend host
	req := httptest.NewRequest(http.MethodGet, "http://localhost:5173/api/v1/ics/feed/test-token.ics", nil)
	req.Header.Set("X-Forwarded-Host", "192.168.1.10:8080")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	// Check body contains the X-Forwarded-Host in UID (not the backend host)
	body := w.Body.String()
	expectedUIDPart := "@192.168.1.10:8080"
	if !strings.Contains(body, expectedUIDPart) {
		t.Errorf("Expected UID to contain '%s' from X-Forwarded-Host, but got:\n%s", expectedUIDPart, body)
	}

	// Make sure it doesn't contain the backend host
	if strings.Contains(body, "@localhost:5173") {
		t.Error("UID should not contain the backend host when X-Forwarded-Host is set")
	}
}

func TestGetFeed_MultipleTimeSlotsPerDay(t *testing.T) {
	// Test case: 3 participants, threshold 2
	// P1: all day (00:00-23:59)
	// P2: until 12:00
	// P3: from 14:00
	// Should generate 2 events: 00:00-12:00 and 14:00-23:59

	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Multi-Slot Calendar",
			Threshold:         2,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6},
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 3,
		},
	}

	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p1Start, p1End := "00:00", "23:59"
	p2Start, p2End := "00:00", "12:00"
	p3Start, p3End := "14:00", "23:59"

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:              date,
					ParticipantName:   "P1",
					StartTime:         &p1Start,
					EndTime:           &p1End,
					Note:              "",
					AvailableCount:    3, // Will be recalculated per slot
					TotalParticipants: 3,
				},
				{
					Date:              date,
					ParticipantName:   "P2",
					StartTime:         &p2Start,
					EndTime:           &p2End,
					Note:              "",
					AvailableCount:    3,
					TotalParticipants: 3,
				},
				{
					Date:              date,
					ParticipantName:   "P3",
					StartTime:         &p3Start,
					EndTime:           &p3End,
					Note:              "",
					AvailableCount:    3,
					TotalParticipants: 3,
				},
			},
		},
	}

	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/test-token.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.GetFeed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Should contain 2 events
	if !strings.Contains(body, "Multi-Slot Calendar #1") {
		t.Error("Expected to find event #1 (morning slot)")
	}
	if !strings.Contains(body, "Multi-Slot Calendar #2") {
		t.Error("Expected to find event #2 (afternoon slot)")
	}

	// Should NOT contain a third event
	if strings.Contains(body, "Multi-Slot Calendar #3") {
		t.Error("Did not expect to find event #3")
	}

	// Check that we have proper time slots - morning event should have 00:00
	// Using floating time (RFC 5545 FORM #1) - no TZID
	if !strings.Contains(body, "DTSTART:20250615T000000") {
		t.Error("Expected morning event to start at 00:00 (floating time)")
	}

	// Check that afternoon event starts at 14:00
	if !strings.Contains(body, "DTSTART:20250615T140000") {
		t.Error("Expected afternoon event to start at 14:00 (floating time)")
	}
}

func TestGetFeed_ContinuousTimeSlot(t *testing.T) {
	// Test case: When there's no gap, should generate single event
	// P1: all day
	// P2: until 12:00
	// P3: from 12:00 (no gap!)
	// Should generate 1 continuous event

	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Continuous Calendar",
			Threshold:         2,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6},
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 3,
		},
	}

	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p1Start, p1End := "00:00", "23:59"
	p2Start, p2End := "00:00", "12:00"
	p3Start, p3End := "12:00", "23:59" // No gap with P2

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:            date,
					ParticipantName: "P1",
					StartTime:       &p1Start,
					EndTime:         &p1End,
				},
				{
					Date:            date,
					ParticipantName: "P2",
					StartTime:       &p2Start,
					EndTime:         &p2End,
				},
				{
					Date:            date,
					ParticipantName: "P3",
					StartTime:       &p3Start,
					EndTime:         &p3End,
				},
			},
		},
	}

	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/test-token.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.GetFeed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Should contain only 1 event (full day since continuous coverage)
	if !strings.Contains(body, "Continuous Calendar #1") {
		t.Error("Expected to find event #1")
	}

	// Should NOT contain a second event
	if strings.Contains(body, "Continuous Calendar #2") {
		t.Error("Should only have 1 event for continuous coverage, not 2")
	}
}

func TestGetFeed_UsesFloatingTimeWithTimezoneHint(t *testing.T) {
	// Test that the ICS feed uses floating time (RFC 5545 FORM #1)
	// with X-WR-TIMEZONE as a hint for calendar clients
	calID := uuid.New()
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Floating Time Calendar",
			Threshold:         1,
			AllowedWeekdays:   []int{0, 1, 2, 3, 4, 5, 6},
			Timezone:          "Asia/Tokyo",
			HolidaysPolicy:    "ignore",
			AllowHolidayEves:  false,
			OwnerID:           uuid.New(),
			TotalParticipants: 1,
		},
	}

	date := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	startTime, endTime := "10:00", "18:00"

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			date: {
				{
					Date:            date,
					ParticipantName: "Taro",
					StartTime:       &startTime,
					EndTime:         &endTime,
				},
			},
		},
	}

	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/test-token.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	handler.GetFeed(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	body := w.Body.String()

	// Should contain X-WR-TIMEZONE as a hint for calendar clients
	if !strings.Contains(body, "X-WR-TIMEZONE:Asia/Tokyo") {
		t.Error("Expected X-WR-TIMEZONE hint for calendar clients")
	}

	// Should NOT have VTIMEZONE component (no DST rules needed)
	if strings.Contains(body, "BEGIN:VTIMEZONE") {
		t.Error("Should NOT have VTIMEZONE component")
	}

	// Event should use floating time (no TZID on events)
	if strings.Contains(body, "DTSTART;TZID=") {
		t.Error("Floating time DTSTART should NOT have TZID parameter")
	}

	// Should have simple DTSTART format: DTSTART:20250615T100000
	if !strings.Contains(body, "DTSTART:20250615T100000") {
		t.Error("Expected DTSTART with floating time format")
	}
}

func TestGetFeed_HolidaysAndHolidayEves(t *testing.T) {
	// Setup mock data
	calID := uuid.New()

	// Test with a calendar that:
	// - Only allows Monday (1) and Friday (5)
	// - Allows holidays (policy: "allow")
	// - Allows holiday eves
	mockCalRepo := &mockCalendarRepository{
		calendar: &repository.Calendar{
			ID:                calID,
			Name:              "Holiday Test Calendar",
			Threshold:         2,
			AllowedWeekdays:   []int{1, 5}, // Monday and Friday only
			Timezone:          "Europe/Paris",
			HolidaysPolicy:    "allow",
			AllowHolidayEves:  true,
			OwnerID:           uuid.New(),
			TotalParticipants: 3,
		},
	}

	startTime := "10:00"
	endTime := "18:00"

	// Create test dates:
	// - Monday June 9, 2025 (regular Monday - should be included)
	// - Wednesday December 25, 2024 (Christmas - holiday on a Wednesday, should be included)
	// - Tuesday December 24, 2024 (Christmas Eve - holiday eve on a Tuesday, should be included)
	// - Thursday June 5, 2025 (regular Thursday - should NOT be included)
	mondayJune9 := time.Date(2025, 6, 9, 0, 0, 0, 0, time.UTC)    // Monday (allowed weekday)
	christmasDay := time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC) // Wednesday (holiday)
	christmasEve := time.Date(2024, 12, 24, 0, 0, 0, 0, time.UTC) // Tuesday (holiday eve)
	thursdayJune5 := time.Date(2025, 6, 5, 0, 0, 0, 0, time.UTC)  // Thursday (not allowed)

	mockAvailRepo := &mockAvailabilityRepository{
		events: map[time.Time][]repository.DateAvailability{
			mondayJune9: {
				{
					Date:              mondayJune9,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
				{
					Date:              mondayJune9,
					ParticipantName:   "Bob",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
			},
			christmasDay: {
				{
					Date:              christmasDay,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
				{
					Date:              christmasDay,
					ParticipantName:   "Charlie",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
			},
			christmasEve: {
				{
					Date:              christmasEve,
					ParticipantName:   "Bob",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
				{
					Date:              christmasEve,
					ParticipantName:   "Charlie",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
			},
			thursdayJune5: {
				{
					Date:              thursdayJune5,
					ParticipantName:   "Alice",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
				{
					Date:              thursdayJune5,
					ParticipantName:   "Bob",
					StartTime:         &startTime,
					EndTime:           &endTime,
					AvailableCount:    2,
					TotalParticipants: 3,
				},
			},
		},
	}

	// Create service and handler
	mockQuota := &mockQuotaChecker{isOverQuota: false}
	icsSvc := service.NewICSService(mockCalRepo, mockAvailRepo, mockQuota, "localhost:8080")
	handler := handlers.NewICSHandler(icsSvc)

	// Create request
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ics/feed/test-token.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("token", "test-token.ics")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	// Create response recorder
	w := httptest.NewRecorder()

	// Call handler
	handler.GetFeed(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code 200, got %d", w.Code)
	}

	// Check body
	body := w.Body.String()

	// Should contain Monday event (allowed weekday)
	if !strings.Contains(body, "Holiday Test Calendar #1") {
		t.Error("Expected to find event #1 (Monday)")
	}

	// Should contain Christmas event (holiday on Wednesday)
	if !strings.Contains(body, "Holiday Test Calendar #2") {
		t.Error("Expected to find event #2 (Christmas Day - holiday)")
	}

	// Should contain Christmas Eve event (holiday eve on Tuesday)
	if !strings.Contains(body, "Holiday Test Calendar #3") {
		t.Error("Expected to find event #3 (Christmas Eve - holiday eve)")
	}

	// Should NOT contain Thursday event (not an allowed weekday, not a holiday or holiday eve)
	// Since we have 3 events, there should be no event #4
	if strings.Contains(body, "Holiday Test Calendar #4") {
		t.Error("Did not expect to find event #4 (Thursday is not allowed)")
	}
}
