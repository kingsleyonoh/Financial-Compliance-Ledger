package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// seedEscalationRule inserts a minimal escalation rule for FK purposes.
func seedEscalationRule(
	t *testing.T, pool *pgxpool.Pool, tenantID uuid.UUID,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO escalation_rules
			(id, tenant_id, name, severity_match, trigger_after_hrs,
			 trigger_status, action, action_config, is_active, priority)
		VALUES ($1, $2, 'test-rule', 'high', 4, 'open', 'notify',
				'{}', true, 1)
	`, id, tenantID)
	require.NoError(t, err, "failed to seed escalation rule")
	return id
}

func TestNotificationStore_Create_Success(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "notif-create")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-notif-1",
		domain.SeverityHigh, domain.StatusOpen)
	ruleID := seedEscalationRule(t, pool, tenantID)
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	notif, err := ns.Create(ctx, tenantID, discID, &ruleID,
		domain.ChannelInApp, "escalation-engine")

	require.NoError(t, err)
	assert.NotEmpty(t, notif.ID)
	assert.Equal(t, tenantID.String(), notif.TenantID)
	assert.Equal(t, discID.String(), notif.DiscrepancyID)
	assert.Equal(t, domain.NotifPending, notif.Status)
	assert.Equal(t, domain.ChannelInApp, notif.Channel)
	assert.Equal(t, "escalation-engine", notif.Recipient)
	assert.Equal(t, 0, notif.Attempts)
	assert.Nil(t, notif.LastAttemptAt)
}

func TestNotificationStore_Create_WithoutRuleID(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "notif-norule")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-notif-norule",
		domain.SeverityHigh, domain.StatusOpen)
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	notif, err := ns.Create(ctx, tenantID, discID, nil,
		domain.ChannelWebhook, "admin@test.com")

	require.NoError(t, err)
	assert.NotEmpty(t, notif.ID)
	assert.Nil(t, notif.EscalationRuleID)
	assert.Equal(t, domain.ChannelWebhook, notif.Channel)
}

func TestNotificationStore_UpdateStatus_Success(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "notif-update")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-notif-upd",
		domain.SeverityHigh, domain.StatusOpen)
	ruleID := seedEscalationRule(t, pool, tenantID)
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	notif, err := ns.Create(ctx, tenantID, discID, &ruleID,
		domain.ChannelInApp, "escalation-engine")
	require.NoError(t, err)

	notifUUID, err := uuid.Parse(notif.ID)
	require.NoError(t, err)

	hubResp := map[string]interface{}{
		"status": "accepted", "id": "hub-123",
	}
	err = ns.UpdateStatus(ctx, notifUUID, domain.NotifSent, hubResp, 1)
	require.NoError(t, err)

	// Verify by re-fetching
	updated, err := ns.GetByID(ctx, notifUUID)
	require.NoError(t, err)
	assert.Equal(t, domain.NotifSent, updated.Status)
	assert.Equal(t, 1, updated.Attempts)
	assert.NotNil(t, updated.LastAttemptAt)
	assert.Equal(t, "accepted", updated.HubResponse["status"])
}

func TestNotificationStore_UpdateStatus_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	ctx := context.Background()
	err := ns.UpdateStatus(ctx, uuid.New(), domain.NotifSent, nil, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestNotificationStore_ListPendingRetries(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "notif-retries")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-notif-retry",
		domain.SeverityHigh, domain.StatusOpen)
	ruleID := seedEscalationRule(t, pool, tenantID)
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Create a failed notification with 1 attempt
	n1, err := ns.Create(ctx, tenantID, discID, &ruleID,
		domain.ChannelInApp, "engine")
	require.NoError(t, err)
	n1UUID, _ := uuid.Parse(n1.ID)
	err = ns.UpdateStatus(ctx, n1UUID, domain.NotifFailed, nil, 1)
	require.NoError(t, err)

	// Create a retrying notification with 2 attempts
	n2, err := ns.Create(ctx, tenantID, discID, nil,
		domain.ChannelWebhook, "admin@test.com")
	require.NoError(t, err)
	n2UUID, _ := uuid.Parse(n2.ID)
	err = ns.UpdateStatus(ctx, n2UUID, domain.NotifRetrying, nil, 2)
	require.NoError(t, err)

	// Create a sent notification (should NOT appear)
	n3, err := ns.Create(ctx, tenantID, discID, nil,
		domain.ChannelEmail, "sent@test.com")
	require.NoError(t, err)
	n3UUID, _ := uuid.Parse(n3.ID)
	err = ns.UpdateStatus(ctx, n3UUID, domain.NotifSent, nil, 1)
	require.NoError(t, err)

	// Create a failed notification with max attempts (should NOT appear)
	n4, err := ns.Create(ctx, tenantID, discID, nil,
		domain.ChannelEmail, "maxed@test.com")
	require.NoError(t, err)
	n4UUID, _ := uuid.Parse(n4.ID)
	err = ns.UpdateStatus(ctx, n4UUID, domain.NotifFailed, nil, 3)
	require.NoError(t, err)

	// Query with maxAttempts=3
	pending, err := ns.ListPendingRetries(ctx, 3)
	require.NoError(t, err)

	// Should return n1 (failed/1) and n2 (retrying/2)
	// n3 is sent, n4 has 3 attempts (>= max)
	ids := make(map[string]bool)
	for _, n := range pending {
		ids[n.ID] = true
	}
	assert.True(t, ids[n1.ID],
		"failed notification with 1 attempt should be included")
	assert.True(t, ids[n2.ID],
		"retrying notification with 2 attempts should be included")
	assert.False(t, ids[n3.ID],
		"sent notification should not be included")
	assert.False(t, ids[n4.ID],
		"maxed out notification should not be included")
}

func TestNotificationStore_GetByRuleAndDiscrepancy(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	tenantID := seedTenant(t, pool, "notif-getrule")
	discID := seedDiscrepancy(t, pool, tenantID, "ext-notif-gr",
		domain.SeverityHigh, domain.StatusOpen)
	ruleID := seedEscalationRule(t, pool, tenantID)
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()

	// Initially no notification exists
	notif, err := ns.GetByRuleAndDiscrepancy(ctx, ruleID, discID)
	require.NoError(t, err)
	assert.Nil(t, notif)

	// Create one
	created, err := ns.Create(ctx, tenantID, discID, &ruleID,
		domain.ChannelInApp, "engine")
	require.NoError(t, err)

	// Now it should be found
	found, err := ns.GetByRuleAndDiscrepancy(ctx, ruleID, discID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, created.ID, found.ID)
}

func TestNotificationStore_GetByRuleAndDiscrepancy_NotFound(t *testing.T) {
	pool := newTestPool(t)
	ns := store.NewNotificationStore(pool)

	ctx := context.Background()
	notif, err := ns.GetByRuleAndDiscrepancy(ctx, uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, notif)
}
