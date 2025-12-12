// WhenTo - Collaborative event calendar for self-hosted environments
// Copyright (C) 2025 WhenTo Contributors
// Licensed under the Business Source License 1.1
// See LICENSE file for details

package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	authModels "github.com/whento/whento/internal/auth/models"
	"github.com/whento/whento/internal/calendar/handlers"
	"github.com/whento/whento/internal/calendar/models"
	"github.com/whento/whento/internal/calendar/repository"
	"github.com/whento/whento/internal/calendar/service"
	"github.com/whento/whento/internal/config"
	"github.com/whento/whento/internal/testutil"
	pkgModels "github.com/whento/pkg/models"
)

// Mock CalendarService implementing service.CalendarRepository and service.ParticipantRepository
type mockCalendarRepository struct {
	calendar        *models.Calendar
	calendars       []*models.Calendar
	participants    []models.Participant
	err             error
	createWithParticipantsCalled bool
}

func (m *mockCalendarRepository) CreateWithParticipants(ctx context.Context, calendar *models.Calendar, participantInputs []repository.ParticipantInput) ([]models.Participant, error) {
	m.createWithParticipantsCalled = true
	if m.err != nil {
		return nil, m.err
	}
	return m.participants, nil
}

func (m *mockCalendarRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Calendar, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calendar == nil {
		return nil, service.ErrCalendarNotFound
	}
	return m.calendar, nil
}

func (m *mockCalendarRepository) GetByOwnerID(ctx context.Context, ownerID uuid.UUID) ([]*models.Calendar, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.calendars, nil
}

func (m *mockCalendarRepository) GetByPublicToken(ctx context.Context, token string) (*models.Calendar, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.calendar == nil {
		return nil, service.ErrCalendarNotFound
	}
	return m.calendar, nil
}

func (m *mockCalendarRepository) Update(ctx context.Context, calendar *models.Calendar) error {
	return m.err
}

func (m *mockCalendarRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockCalendarRepository) RegenerateToken(ctx context.Context, id uuid.UUID, tokenType, newToken string) error {
	return m.err
}

type mockParticipantRepository struct {
	participant  *models.Participant
	participants []models.Participant
	err          error
}

func (m *mockParticipantRepository) Create(ctx context.Context, participant *models.Participant) error {
	return m.err
}

func (m *mockParticipantRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.Participant, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.participant == nil {
		return nil, service.ErrParticipantNotFound
	}
	return m.participant, nil
}

func (m *mockParticipantRepository) GetByCalendarID(ctx context.Context, calendarID uuid.UUID) ([]models.Participant, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.participants, nil
}

func (m *mockParticipantRepository) Update(ctx context.Context, id uuid.UUID, name string) error {
	return m.err
}

func (m *mockParticipantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockParticipantRepository) SetEmailAsVerified(ctx context.Context, participantID uuid.UUID, locale string) error {
	return m.err
}

type mockCache struct{}

func (m *mockCache) Get(ctx context.Context, key string, dest interface{}) error {
	return errors.New("not found")
}

func (m *mockCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return nil
}

func (m *mockCache) Delete(ctx context.Context, keys ...string) error {
	return nil
}

func (m *mockCache) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (m *mockCache) IsEnabled() bool {
	return true
}

type mockQuotaService struct {
	canCreate    bool
	userLimit    int
	serverLimit  int
	usage        int
	isOverQuota  bool
	err          error
}

func (m *mockQuotaService) CanCreateCalendar(ctx context.Context, userID uuid.UUID) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.canCreate, nil
}

func (m *mockQuotaService) GetUserLimit(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.userLimit, nil
}

func (m *mockQuotaService) GetServerLimit(ctx context.Context) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.serverLimit, nil
}

func (m *mockQuotaService) GetCurrentUsage(ctx context.Context, userID uuid.UUID) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.usage, nil
}

func (m *mockQuotaService) GetServerUsage(ctx context.Context) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.usage, nil
}

func (m *mockQuotaService) IsOverQuota(ctx context.Context, userID uuid.UUID) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.isOverQuota, nil
}

type mockUserRepository struct {
	user *authModels.User
	err  error
}

func (m *mockUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*authModels.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.user == nil {
		return &authModels.User{
			TimestampedEntity: pkgModels.TimestampedEntity{Entity: pkgModels.Entity{ID: id}},
			EmailVerified:     true,
		}, nil
	}
	return m.user, nil
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*authModels.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) Create(ctx context.Context, user *authModels.User) error {
	return m.err
}

func (m *mockUserRepository) Update(ctx context.Context, user *authModels.User) error {
	return m.err
}

func (m *mockUserRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) SetEmailAsVerified(ctx context.Context, id uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) SetVerificationToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	return m.err
}

func (m *mockUserRepository) GetByVerificationToken(ctx context.Context, token string) (*authModels.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) ClearVerificationToken(ctx context.Context, userID uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) UpdatePassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	return m.err
}

func (m *mockUserRepository) UpdateRole(ctx context.Context, userID uuid.UUID, role string) error {
	return m.err
}

func (m *mockUserRepository) ListAll(ctx context.Context) ([]authModels.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) SetPasswordResetToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	return m.err
}

func (m *mockUserRepository) GetByPasswordResetToken(ctx context.Context, token string) (*authModels.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) ClearPasswordResetToken(ctx context.Context, userID uuid.UUID) error {
	return m.err
}

func (m *mockUserRepository) SetMagicLinkToken(ctx context.Context, userID uuid.UUID, token string, expiresAt time.Time) error {
	return m.err
}

func (m *mockUserRepository) GetByMagicLinkToken(ctx context.Context, token string) (*authModels.User, error) {
	return nil, errors.New("not implemented")
}

func (m *mockUserRepository) ClearMagicLinkToken(ctx context.Context, userID uuid.UUID) error {
	return m.err
}

// Verify interface implementations at compile time
var _ service.CalendarRepository = (*mockCalendarRepository)(nil)
var _ service.ParticipantRepository = (*mockParticipantRepository)(nil)

// Test CreateCalendar
func TestCalendarHandler_CreateCalendar_Success(t *testing.T) {
	ownerID := uuid.New()

	mockCalRepo := &mockCalendarRepository{
		participants: []models.Participant{
			{Name: "Alice"},
			{Name: "Bob"},
		},
	}
	mockPartRepo := &mockParticipantRepository{}
	mockCache := &mockCache{}
	mockQuota := &mockQuotaService{canCreate: true}

	calendarSvc := service.NewCalendarService(mockCalRepo, mockPartRepo, nil, mockCache)
	cfg := &config.Config{Email: config.EmailConfig{VerificationEnabled: false}}
	handler := handlers.NewCalendarHandler(calendarSvc, mockQuota, nil, cfg)

	reqBody := map[string]interface{}{
		"name":        "Team Meeting",
		"description": "Weekly sync",
		"threshold":   2,
		"participants": []string{"Alice", "Bob"},
	}

	req := testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", reqBody)
	req = testutil.WithAuth(req, ownerID.String(), "user")
	w := httptest.NewRecorder()

	handler.CreateCalendar(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	if !mockCalRepo.createWithParticipantsCalled {
		t.Error("Expected CreateWithParticipants to be called")
	}
}

func TestCalendarHandler_CreateCalendar_QuotaExceeded(t *testing.T) {
	ownerID := uuid.New()

	mockCalRepo := &mockCalendarRepository{}
	mockPartRepo := &mockParticipantRepository{}
	mockCache := &mockCache{}
	mockQuota := &mockQuotaService{canCreate: false} // Quota exceeded

	calendarSvc := service.NewCalendarService(mockCalRepo, mockPartRepo, nil, mockCache)
	cfg := &config.Config{Email: config.EmailConfig{VerificationEnabled: false}}
	handler := handlers.NewCalendarHandler(calendarSvc, mockQuota, nil, cfg)

	reqBody := map[string]interface{}{
		"name": "Test Calendar",
	}

	req := testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", reqBody)
	req = testutil.WithAuth(req, ownerID.String(), "user")
	w := httptest.NewRecorder()

	handler.CreateCalendar(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCalendarHandler_CreateCalendar_Unauthorized(t *testing.T) {
	mockCalRepo := &mockCalendarRepository{}
	mockPartRepo := &mockParticipantRepository{}
	mockCache := &mockCache{}
	mockQuota := &mockQuotaService{canCreate: true}

	calendarSvc := service.NewCalendarService(mockCalRepo, mockPartRepo, nil, mockCache)
	cfg := &config.Config{Email: config.EmailConfig{VerificationEnabled: false}}
	handler := handlers.NewCalendarHandler(calendarSvc, mockQuota, nil, cfg)

	reqBody := map[string]interface{}{
		"name": "Test Calendar",
	}

	req := testutil.MakeJSONRequest(http.MethodPost, "/api/v1/calendars", reqBody)
	// No auth context
	w := httptest.NewRecorder()

	handler.CreateCalendar(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", w.Code)
	}
}

// More tests to be added: GetCalendar, ListMyCalendars, UpdateCalendar, DeleteCalendar, RegenerateToken, GetPublicCalendar
