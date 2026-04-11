package engine_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// seedDiscrepancyAt inserts a test discrepancy with a specified created_at.
func seedDiscrepancyAt(
	t *testing.T, pool *pgxpool.Pool,
	tenantID uuid.UUID, externalID, severity, status string,
	createdAt time.Time,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metadata, _ := json.Marshal(map[string]interface{}{})
	_, err := pool.Exec(ctx, `
		INSERT INTO discrepancies
			(id, tenant_id, external_id, source_system, discrepancy_type,
			 severity, status, title, currency, metadata,
			 first_detected_at, created_at)
		VALUES ($1, $2, $3, 'test-system', 'mismatch', $4, $5,
				'Test Discrepancy', 'USD', $6, $7, $7)
	`, id, tenantID, externalID, severity, status, metadata, createdAt)
	require.NoError(t, err, "failed to seed discrepancy with timestamp")
	return id
}

// seedRule inserts a test escalation rule.
func seedRule(
	t *testing.T, rs *store.RuleStore, tenantID uuid.UUID,
	name, severityMatch, triggerStatus, action string,
	triggerAfterHrs, priority int,
	actionConfig map[string]interface{},
) string {
	t.Helper()
	ctx := context.Background()
	rule := &domain.EscalationRule{
		TenantID:        tenantID.String(),
		Name:            name,
		SeverityMatch:   severityMatch,
		TriggerAfterHrs: triggerAfterHrs,
		TriggerStatus:   triggerStatus,
		Action:          action,
		ActionConfig:    actionConfig,
		IsActive:        true,
		Priority:        priority,
	}
	created, err := rs.Create(ctx, tenantID, rule)
	require.NoError(t, err, "failed to seed rule")
	return created.ID
}

// newTestEscalationEngine creates an escalation engine for testing.
func newTestEscalationEngine(
	t *testing.T, pool *pgxpool.Pool,
) *engine.EscalationEngine {
	t.Helper()
	rs := store.NewRuleStore(pool)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()
	return engine.NewEscalationEngine(
		rs, ds, es, pool, logger, 15*time.Minute)
}

func TestEscalationEngine_Evaluate_EscalateAction(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	es := store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-escalate")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	// Create a discrepancy old enough to trigger
	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-esc-001",
		domain.SeverityHigh, domain.StatusInvestigating, sixHoursAgo)

	// Create an escalation rule: escalate high+investigating after 4 hrs
	seedRule(t, rs, tenantID,
		"Escalate High", domain.SeverityHigh,
		domain.StatusInvestigating, domain.ActionEscalate, 4, 1,
		map[string]interface{}{"new_severity": "critical"})

	// Run evaluation
	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Verify status changed to escalated
	disc, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusEscalated, disc.Status)

	// Verify escalation event was appended
	events, err := es.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.EventEscalated, events[0].EventType)
	assert.Equal(t, domain.ActorEscalation, events[0].ActorType)
}

func TestEscalationEngine_Evaluate_AutoCloseAction(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	es := store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-autoclose")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	tenHoursAgo := now.Add(-10 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-ac-001",
		domain.SeverityLow, domain.StatusOpen, tenHoursAgo)

	seedRule(t, rs, tenantID,
		"Auto-Close Low", domain.SeverityLow,
		domain.StatusOpen, domain.ActionAutoClose, 8, 1,
		map[string]interface{}{})

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	disc, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAutoClosed, disc.Status)

	events, err := es.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, domain.EventAutoClosed, events[0].EventType)
}

func TestEscalationEngine_Evaluate_NotifyAction(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	_ = store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-notify")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-noti-001",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	seedRule(t, rs, tenantID,
		"Notify Critical", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{"channel": "#alerts"})

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Notify should NOT change status (hub client is nil, just logs)
	disc, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOpen, disc.Status)

	// Verify notification was logged to notification_log table
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_log
		WHERE tenant_id = $1 AND discrepancy_id = $2
	`, tenantID, discID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"notify action should insert a notification_log entry")
}

func TestEscalationEngine_Evaluate_Dedup_NoDoubleFire(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-dedup")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-dd-001",
		domain.SeverityCritical, domain.StatusOpen, sixHoursAgo)

	ruleID := seedRule(t, rs, tenantID,
		"Notify Critical Dedup", domain.SeverityCritical,
		domain.StatusOpen, domain.ActionNotify, 4, 1,
		map[string]interface{}{"channel": "#alerts"})

	// Run evaluation twice
	err := eng.Evaluate(ctx)
	require.NoError(t, err)
	err = eng.Evaluate(ctx)
	require.NoError(t, err)

	// Should only have 1 notification_log entry, not 2
	ruleUUID, err := uuid.Parse(ruleID)
	require.NoError(t, err)
	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM notification_log
		WHERE tenant_id = $1 AND discrepancy_id = $2
		  AND escalation_rule_id = $3
	`, tenantID, discID, ruleUUID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"notify rule should fire at most once per discrepancy")
}

func TestEscalationEngine_Evaluate_WildcardSeverity(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	es := store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-wildcard")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discIDHigh := seedDiscrepancyAt(t, pool, tenantID, "ext-wc-high",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)
	discIDLow := seedDiscrepancyAt(t, pool, tenantID, "ext-wc-low",
		domain.SeverityLow, domain.StatusOpen, sixHoursAgo)

	// Wildcard severity — matches any
	seedRule(t, rs, tenantID,
		"Auto-Close All Wildcard", "*",
		domain.StatusOpen, domain.ActionAutoClose, 4, 1,
		map[string]interface{}{})

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	// Both should be auto_closed
	dHigh, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discIDHigh)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAutoClosed, dHigh.Status)

	dLow, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discIDLow)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAutoClosed, dLow.Status)

	// Verify events
	evHigh, err := es.ListByDiscrepancy(ctx, tenantID, discIDHigh)
	require.NoError(t, err)
	assert.Len(t, evHigh, 1)

	evLow, err := es.ListByDiscrepancy(ctx, tenantID, discIDLow)
	require.NoError(t, err)
	assert.Len(t, evLow, 1)
}

func TestEscalationEngine_Evaluate_SkipsInactiveTenants(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	es := store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	// Create inactive tenant
	inactiveID := uuid.New()
	apiKey := "test-api-key-" + inactiveID.String()[:8]
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, false)
	`, inactiveID, "esc-inactive-tenant", apiKey)
	require.NoError(t, err)
	t.Cleanup(func() { cleanupTenantData(t, pool, inactiveID) })

	now := time.Now().UTC()
	sixHoursAgo := now.Add(-6 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, inactiveID, "ext-inact-001",
		domain.SeverityHigh, domain.StatusOpen, sixHoursAgo)

	seedRule(t, rs, inactiveID,
		"Inactive Rule", domain.SeverityHigh,
		domain.StatusOpen, domain.ActionAutoClose, 4, 1,
		map[string]interface{}{})

	err = eng.Evaluate(ctx)
	require.NoError(t, err)

	// Status should NOT change (inactive tenant skipped)
	disc, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, inactiveID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOpen, disc.Status)

	events, err := es.ListByDiscrepancy(ctx, inactiveID, discID)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestEscalationEngine_Evaluate_RecentDiscrepancyNotTriggered(t *testing.T) {
	pool := newTestPool(t)
	rs := store.NewRuleStore(pool)
	es := store.NewEventStore(pool)
	eng := newTestEscalationEngine(t, pool)

	tenantID := seedTenant(t, pool, "esc-recent")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	now := time.Now().UTC()
	oneHourAgo := now.Add(-1 * time.Hour)

	discID := seedDiscrepancyAt(t, pool, tenantID, "ext-recent-001",
		domain.SeverityHigh, domain.StatusOpen, oneHourAgo)

	// Rule triggers after 4 hours, discrepancy is only 1 hour old
	seedRule(t, rs, tenantID,
		"Escalate After 4hrs", domain.SeverityHigh,
		domain.StatusOpen, domain.ActionAutoClose, 4, 1,
		map[string]interface{}{})

	err := eng.Evaluate(ctx)
	require.NoError(t, err)

	disc, err := store.NewDiscrepancyStore(pool).GetByID(
		ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusOpen, disc.Status,
		"recent discrepancy should not be triggered")

	events, err := es.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestEscalationEngine_Start_StopsOnContextCancel(t *testing.T) {
	pool := newTestPool(t)
	eng := newTestEscalationEngine(t, pool)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		eng.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	// Should stop within a reasonable time
	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not stop after context cancellation")
	}
}
