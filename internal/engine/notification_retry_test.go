package engine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// seedNotification inserts a notification_log entry for testing.
func seedNotification(
	t *testing.T, ns *store.NotificationStore,
	tenantID, discID uuid.UUID, ruleID *uuid.UUID,
	status string, attempts int,
) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	notif, err := ns.Create(ctx, tenantID, discID, ruleID,
		domain.ChannelInApp, "escalation-engine")
	require.NoError(t, err)

	id, err := uuid.Parse(notif.ID)
	require.NoError(t, err)

	if status != domain.NotifPending || attempts > 0 {
		err = ns.UpdateStatus(ctx, id, status, nil, attempts)
		require.NoError(t, err)
	}
	return id
}

func TestNotificationRetrier_RetryPending_SuccessfulRetry(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "retry-success")
	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-retry-s",
		domain.SeverityHigh, domain.StatusOpen, time.Now().UTC())
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	// Create a failed notification
	notifID := seedNotification(t, ns, tenantID, discID, nil,
		domain.NotifFailed, 1)

	// Mock hub server that accepts
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status": "accepted",
				"id":     "hub-retry-123",
			})
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 3,
	}

	hubClient := notify.NewHubClientWithBackoff(
		cfg, zerolog.Nop(), []time.Duration{time.Millisecond})
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	retrier := engine.NewNotificationRetrier(ns, hubClient, 3, logger)

	ctx := context.Background()
	err := retrier.RetryPending(ctx)
	require.NoError(t, err)

	// Verify notification is now sent
	updated, err := ns.GetByID(ctx, notifID)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifSent, updated.Status)
	assert.Equal(t, 2, updated.Attempts) // was 1, now 2
	assert.NotNil(t, updated.HubResponse)
}

func TestNotificationRetrier_RetryPending_FailedRetry(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "retry-fail")
	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-retry-f",
		domain.SeverityHigh, domain.StatusOpen, time.Now().UTC())
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	// Create a failed notification with 1 attempt
	notifID := seedNotification(t, ns, tenantID, discID, nil,
		domain.NotifFailed, 1)

	// Mock hub server that always fails
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 1, // Only 1 attempt per retry round
	}

	hubClient := notify.NewHubClientWithBackoff(
		cfg, zerolog.Nop(), []time.Duration{time.Millisecond})
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	retrier := engine.NewNotificationRetrier(ns, hubClient, 3, logger)

	ctx := context.Background()
	err := retrier.RetryPending(ctx)
	require.NoError(t, err) // RetryPending itself doesn't error

	// Verify notification is now retrying with incremented attempts
	updated, err := ns.GetByID(ctx, notifID)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifRetrying, updated.Status)
	assert.Equal(t, 2, updated.Attempts) // was 1, now 2
}

func TestNotificationRetrier_RetryPending_MaxAttemptsExhausted(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "retry-maxed")
	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-retry-m",
		domain.SeverityHigh, domain.StatusOpen, time.Now().UTC())
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	// Create a retrying notification with 2 of 3 attempts
	notifID := seedNotification(t, ns, tenantID, discID, nil,
		domain.NotifRetrying, 2)

	// Mock hub that always fails
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 1,
	}

	hubClient := notify.NewHubClientWithBackoff(
		cfg, zerolog.Nop(), []time.Duration{time.Millisecond})
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	retrier := engine.NewNotificationRetrier(ns, hubClient, 3, logger)

	ctx := context.Background()
	err := retrier.RetryPending(ctx)
	require.NoError(t, err)

	// Verify notification is now failed with 3 attempts (maxed out)
	updated, err := ns.GetByID(ctx, notifID)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifFailed, updated.Status)
	assert.Equal(t, 3, updated.Attempts)
}

func TestNotificationRetrier_RetryPending_NoPending(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	cfg := &config.Config{
		NotificationHubEnabled: false,
	}
	hubClient := notify.NewHubClient(cfg, zerolog.Nop())
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	retrier := engine.NewNotificationRetrier(ns, hubClient, 3, logger)

	ctx := context.Background()
	err := retrier.RetryPending(ctx)
	require.NoError(t, err) // No error when nothing to retry
}

func TestNotificationRetrier_Start_StopsOnContextCancel(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	cfg := &config.Config{
		NotificationHubEnabled: false,
	}
	hubClient := notify.NewHubClient(cfg, zerolog.Nop())
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	retrier := engine.NewNotificationRetrier(ns, hubClient, 3, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		retrier.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not stop after context cancellation")
	}
}
