package engine_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func TestEscalationEngine_NotifyWithHubClient_Success(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "esc-hub-success")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-hub-s",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	seedRule(t, rs, tenantID,
		"Notify Via Hub", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{"channel": "#alerts"})

	// Mock hub server
	var hubCalled int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&hubCalled, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status": "accepted",
				"id":     "hub-esc-123",
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
	eng := engine.NewEscalationEngine(
		rs, ds, es, pool, logger, 15*time.Minute)
	eng.WithNotifications(ns, hubClient)

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Hub was called
	assert.Equal(t, int32(1), atomic.LoadInt32(&hubCalled))

	// Notification log was created and updated to "sent"
	var status string
	var attempts int
	err = pool.QueryRow(ctx, `
		SELECT status, attempts FROM notification_log
		WHERE tenant_id = $1 AND discrepancy_id = $2
	`, tenantID, discID).Scan(&status, &attempts)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifSent, status)
	assert.Equal(t, 1, attempts)
}

func TestEscalationEngine_NotifyWithHubClient_HubDown(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "esc-hub-down")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-hub-d",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	seedRule(t, rs, tenantID,
		"Notify Hub Down", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{})

	// Mock hub that always 500s
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
	eng := engine.NewEscalationEngine(
		rs, ds, es, pool, logger, 15*time.Minute)
	eng.WithNotifications(ns, hubClient)

	// Evaluate should NOT panic or return error — hub downtime is
	// handled gracefully
	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Notification log should be "failed"
	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM notification_log
		WHERE tenant_id = $1 AND discrepancy_id = $2
	`, tenantID, discID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifFailed, status)
}

func TestEscalationEngine_NotifyWithHubClient_Disabled(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "esc-hub-disabled")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-hub-dis",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	seedRule(t, rs, tenantID,
		"Notify Disabled", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{})

	cfg := &config.Config{
		NotificationHubEnabled: false,
		MaxNotificationRetries: 3,
	}
	hubClient := notify.NewHubClient(cfg, zerolog.Nop())

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()
	eng := engine.NewEscalationEngine(
		rs, ds, es, pool, logger, 15*time.Minute)
	eng.WithNotifications(ns, hubClient)

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Notification log should exist with pending status (hub returned
	// "skipped", so we keep it as pending)
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_log
		WHERE tenant_id = $1 AND discrepancy_id = $2
	`, tenantID, discID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"notification_log entry should be created even when hub disabled")
}

func TestEscalationEngine_NotifyDedup_WithNotificationStore(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "esc-hub-dedup")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-hub-dd",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)
	_ = discID

	ruleID := seedRule(t, rs, tenantID,
		"Notify Dedup Hub", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{})
	_ = ruleID

	cfg := &config.Config{
		NotificationHubEnabled: false,
		MaxNotificationRetries: 3,
	}
	hubClient := notify.NewHubClient(cfg, zerolog.Nop())

	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()
	eng := engine.NewEscalationEngine(
		rs, ds, es, pool, logger, 15*time.Minute)
	eng.WithNotifications(ns, hubClient)

	// Evaluate twice
	err := eng.Evaluate(ctx)
	require.NoError(t, err)
	err = eng.Evaluate(ctx)
	require.NoError(t, err)

	// Should only have 1 notification
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_log
		WHERE tenant_id = $1
	`, tenantID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"should only create 1 notification log entry, dedup prevents double")
}
