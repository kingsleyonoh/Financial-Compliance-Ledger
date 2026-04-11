// Package api contains end-to-end integration tests that exercise the
// full Financial Compliance Ledger pipeline: NATS ingestion, discrepancy
// creation, workflow actions, escalation evaluation, and report
// generation — all against real PostgreSQL and real NATS.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/report"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// --- Test helpers ---

func testDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

func testNATSURL() string {
	u := os.Getenv("NATS_URL")
	if u == "" {
		u = "nats://localhost:4222"
	}
	return u
}

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN()

	err := store.RunMigrations(dsn, "file://../../../migrations")
	require.NoError(t, err, "migrations failed")

	pool, err := store.NewPostgresPool(dsn)
	require.NoError(t, err, "pool creation failed")

	t.Cleanup(func() { pool.Close() })
	return pool
}

func seedE2ETenant(
	t *testing.T, pool *pgxpool.Pool, name string,
) uuid.UUID {
	t.Helper()
	id := uuid.New()
	apiKey := fmt.Sprintf("e2e-api-key-%s", id.String()[:8])
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, true)
	`, id, name, apiKey)
	require.NoError(t, err, "failed to seed e2e tenant")
	return id
}

func cleanupE2ETenant(t *testing.T, pool *pgxpool.Pool, ids ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, tid := range ids {
		_, _ = pool.Exec(ctx,
			`DELETE FROM notification_log WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx,
			`DELETE FROM ledger_events WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx,
			`DELETE FROM escalation_rules WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx,
			`DELETE FROM reports WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx,
			`DELETE FROM discrepancies WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx,
			`DELETE FROM tenants WHERE id = $1`, tid)
	}
}

func waitFor(
	t *testing.T, timeout time.Duration, check func() bool,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

// --- Full pipeline integration test ---

func TestFullPipeline_NATSIngestionToReport(t *testing.T) {
	// -------------------------------------------------------
	// STEP 1: Setup — pool, stores, NATS connection
	// -------------------------------------------------------
	pool := setupPool(t)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()

	tenantID := seedE2ETenant(t, pool, "e2e-pipeline-tenant")
	t.Cleanup(func() { cleanupE2ETenant(t, pool, tenantID) })

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	ruleStore := store.NewRuleStore(pool)
	reportStore := store.NewReportStore(pool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// -------------------------------------------------------
	// STEP 2: Start NATS ingestion consumer
	// -------------------------------------------------------
	subject := fmt.Sprintf("test.e2e.%s", uuid.New().String()[:8])
	consumerName := fmt.Sprintf("e2e-%s", uuid.New().String()[:8])

	nc, err := nats.Connect(testNATSURL())
	require.NoError(t, err, "NATS connect failed")
	t.Cleanup(func() { nc.Close() })

	js, err := nc.JetStream()
	require.NoError(t, err)

	streamName := "E2E_" + uuid.New().String()[:8]
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	require.NoError(t, err, "create test stream failed")
	t.Cleanup(func() { _ = js.DeleteStream(streamName) })

	cfg := &config.Config{
		NATSURL:          testNATSURL(),
		NATSSubject:      subject,
		NATSConsumerName: consumerName,
	}
	ing, err := engine.NewIngestion(
		cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err, "create ingestion failed")

	err = ing.Start(ctx)
	require.NoError(t, err, "start ingestion failed")
	t.Cleanup(func() { _ = ing.Stop() })

	// -------------------------------------------------------
	// STEP 3: Publish a discrepancy event via NATS
	// -------------------------------------------------------
	externalID := fmt.Sprintf("e2e-%s", uuid.New().String()[:8])
	envelope := map[string]interface{}{
		"event_type": "discrepancy.detected",
		"source":     "e2e-test",
		"tenant_id":  tenantID.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]interface{}{
			"external_id":      externalID,
			"source_system":    "e2e-reconciler",
			"discrepancy_type": "mismatch",
			"severity":         "high",
			"title":            "E2E amount mismatch",
			"description":      "Expected 1000, got 950",
			"amount_expected":  1000.00,
			"amount_actual":    950.00,
			"currency":         "USD",
			"metadata":         map[string]interface{}{"test": true},
		},
	}
	envBytes, err := json.Marshal(envelope)
	require.NoError(t, err)
	_, err = js.Publish(subject, envBytes)
	require.NoError(t, err, "publish NATS event failed")

	// -------------------------------------------------------
	// STEP 4: Verify ingestion — discrepancy appears
	// -------------------------------------------------------
	var disc *domain.Discrepancy
	waitFor(t, 5*time.Second, func() bool {
		d, err := discStore.GetByExternalID(ctx, tenantID, externalID)
		if err == nil && d != nil {
			disc = d
			return true
		}
		return false
	})
	require.NotNil(t, disc, "discrepancy should be ingested")
	assert.Equal(t, externalID, disc.ExternalID)
	assert.Equal(t, domain.StatusOpen, disc.Status)
	assert.Equal(t, domain.SeverityHigh, disc.Severity)
	assert.Equal(t, "mismatch", disc.DiscrepancyType)

	discID, err := uuid.Parse(disc.ID)
	require.NoError(t, err)

	// -------------------------------------------------------
	// STEP 5: Verify ledger event — discrepancy.received
	// -------------------------------------------------------
	events, err := eventStore.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Len(t, events, 1, "should have exactly 1 event after ingestion")
	assert.Equal(t, domain.EventReceived, events[0].EventType)
	assert.Equal(t, domain.ActorSystem, events[0].ActorType)

	// -------------------------------------------------------
	// STEP 6: Workflow — acknowledge, investigate, resolve
	// -------------------------------------------------------

	// Acknowledge
	err = discStore.UpdateStatus(
		ctx, tenantID, discID, domain.StatusAcknowledged, nil)
	require.NoError(t, err)
	ackEvent := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventAcknowledged, "e2e-user", domain.ActorUser,
		map[string]interface{}{"action": "acknowledged"},
	)
	_, err = eventStore.Append(ctx, tenantID, ackEvent)
	require.NoError(t, err)

	// Investigate
	err = discStore.UpdateStatus(
		ctx, tenantID, discID, domain.StatusInvestigating, nil)
	require.NoError(t, err)
	invEvent := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventInvestigationStarted, "e2e-user", domain.ActorUser,
		map[string]interface{}{"notes": "investigating root cause"},
	)
	_, err = eventStore.Append(ctx, tenantID, invEvent)
	require.NoError(t, err)

	// Add a note
	noteEvent := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventNoteAdded, "e2e-user", domain.ActorUser,
		map[string]interface{}{"note": "found the discrepancy source"},
	)
	_, err = eventStore.Append(ctx, tenantID, noteEvent)
	require.NoError(t, err)

	// Resolve
	now := time.Now().UTC()
	err = discStore.UpdateStatus(
		ctx, tenantID, discID, domain.StatusResolved, &now)
	require.NoError(t, err)
	resolveEvent := domain.NewLedgerEvent(
		tenantID.String(), disc.ID,
		domain.EventResolved, "e2e-user", domain.ActorUser,
		map[string]interface{}{
			"resolution_type": "manual_adjustment",
			"notes":           "adjusted ledger entry",
		},
	)
	_, err = eventStore.Append(ctx, tenantID, resolveEvent)
	require.NoError(t, err)

	// Verify full event timeline
	allEvents, err := eventStore.ListByDiscrepancy(ctx, tenantID, discID)
	require.NoError(t, err)
	require.Len(t, allEvents, 5,
		"should have 5 events: received, ack, investigate, note, resolve")
	assert.Equal(t, domain.EventReceived, allEvents[0].EventType)
	assert.Equal(t, domain.EventAcknowledged, allEvents[1].EventType)
	assert.Equal(t, domain.EventInvestigationStarted, allEvents[2].EventType)
	assert.Equal(t, domain.EventNoteAdded, allEvents[3].EventType)
	assert.Equal(t, domain.EventResolved, allEvents[4].EventType)

	// Verify final discrepancy state
	resolved, err := discStore.GetByID(ctx, tenantID, discID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusResolved, resolved.Status)
	assert.NotNil(t, resolved.ResolvedAt)

	// -------------------------------------------------------
	// STEP 7: Escalation — create rule, verify it fires
	// -------------------------------------------------------

	// Create a second discrepancy for escalation testing
	// (the first one is resolved, so it won't match)
	externalID2 := fmt.Sprintf("e2e-esc-%s", uuid.New().String()[:8])
	env2 := map[string]interface{}{
		"event_type": "discrepancy.detected",
		"source":     "e2e-test",
		"tenant_id":  tenantID.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]interface{}{
			"external_id":      externalID2,
			"source_system":    "e2e-reconciler",
			"discrepancy_type": "missing",
			"severity":         "critical",
			"title":            "E2E critical unmatched",
			"description":      "Unmatched payment gateway event",
			"amount_expected":  5000.00,
			"amount_actual":    0.0,
			"currency":         "USD",
			"metadata":         map[string]interface{}{},
		},
	}
	envBytes2, err := json.Marshal(env2)
	require.NoError(t, err)
	_, err = js.Publish(subject, envBytes2)
	require.NoError(t, err)

	var disc2 *domain.Discrepancy
	waitFor(t, 5*time.Second, func() bool {
		d, err := discStore.GetByExternalID(ctx, tenantID, externalID2)
		if err == nil && d != nil {
			disc2 = d
			return true
		}
		return false
	})
	require.NotNil(t, disc2)

	// Backdate the second discrepancy so the escalation rule triggers
	disc2ID, err := uuid.Parse(disc2.ID)
	require.NoError(t, err)
	fiveHoursAgo := time.Now().UTC().Add(-5 * time.Hour)
	_, err = pool.Exec(ctx,
		`UPDATE discrepancies SET created_at = $1 WHERE id = $2`,
		fiveHoursAgo, disc2ID)
	require.NoError(t, err)

	// Create escalation rule: auto-close critical+open after 4 hours
	rule := &domain.EscalationRule{
		Name:            "E2E Auto-Close Critical",
		SeverityMatch:   domain.SeverityCritical,
		TriggerAfterHrs: 4,
		TriggerStatus:   domain.StatusOpen,
		Action:          domain.ActionAutoClose,
		ActionConfig:    map[string]interface{}{},
		IsActive:        true,
		Priority:        1,
	}
	_, err = ruleStore.Create(ctx, tenantID, rule)
	require.NoError(t, err)

	// Create and run escalation engine
	escEngine := engine.NewEscalationEngine(
		ruleStore, discStore, eventStore, pool, logger, 15*time.Minute)
	err = escEngine.Evaluate(ctx)
	require.NoError(t, err)

	// Verify disc2 was auto-closed
	disc2After, err := discStore.GetByID(ctx, tenantID, disc2ID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAutoClosed, disc2After.Status,
		"escalation should auto-close the critical discrepancy")

	// Verify auto_closed event exists
	disc2Events, err := eventStore.ListByDiscrepancy(
		ctx, tenantID, disc2ID)
	require.NoError(t, err)
	hasAutoClosedEvent := false
	for _, ev := range disc2Events {
		if ev.EventType == domain.EventAutoClosed {
			hasAutoClosedEvent = true
			break
		}
	}
	assert.True(t, hasAutoClosedEvent,
		"should have discrepancy.auto_closed event")

	// -------------------------------------------------------
	// STEP 8: Report generation — create and verify
	// -------------------------------------------------------
	tmpDir, err := os.MkdirTemp("", "e2e-reports-*")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	reportGen := report.NewReportGenerator(
		discStore, eventStore, reportStore,
		tmpDir, 10000, logger,
	)

	rpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "E2E Test Daily Summary",
		Parameters: map[string]interface{}{
			"date_from": time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
			"date_to":   time.Now().Format("2006-01-02"),
		},
		Status: domain.ReportStatusPending,
	}
	createdReport, err := reportStore.Create(ctx, tenantID, rpt)
	require.NoError(t, err)

	err = reportGen.Generate(ctx, tenantID, createdReport)
	require.NoError(t, err)

	// Verify report was completed
	reportID, err := uuid.Parse(createdReport.ID)
	require.NoError(t, err)
	finalReport, err := reportStore.GetByID(ctx, tenantID, reportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCompleted, finalReport.Status)
	require.NotNil(t, finalReport.FilePath,
		"completed report should have a file path")

	// Verify the file exists on disk
	_, err = os.Stat(*finalReport.FilePath)
	assert.NoError(t, err, "report file should exist on disk")

	// Verify file is not empty
	info, err := os.Stat(*finalReport.FilePath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0),
		"report file should not be empty")

	// -------------------------------------------------------
	// STEP 9: Verify aggregate stats
	// -------------------------------------------------------
	eventCounts, err := eventStore.CountByType(ctx, tenantID)
	require.NoError(t, err)
	assert.Greater(t, eventCounts[domain.EventReceived], 0,
		"should have received events")

	// Verify the report output contains tenant discrepancies
	reportContent, err := os.ReadFile(*finalReport.FilePath)
	require.NoError(t, err)
	assert.Contains(t, string(reportContent), "E2E",
		"report should contain E2E-related content")

	// -------------------------------------------------------
	// STEP 10: Verify report cleanup on old reports
	// -------------------------------------------------------

	// Create an old completed report for cleanup testing
	oldRpt := &domain.Report{
		ReportType: domain.ReportTypeDailySummary,
		Title:      "Old Cleanup Report",
		Parameters: map[string]interface{}{},
		Status:     domain.ReportStatusPending,
	}
	oldCreated, err := reportStore.Create(ctx, tenantID, oldRpt)
	require.NoError(t, err)

	oldReportID, err := uuid.Parse(oldCreated.ID)
	require.NoError(t, err)

	oldFile := filepath.Join(tmpDir, "old-report.html")
	require.NoError(t, os.WriteFile(oldFile, []byte("old content"), 0o644))
	oldSize := int64(len("old content"))
	err = reportStore.UpdateStatus(ctx, tenantID, oldReportID,
		domain.ReportStatusCompleted, &oldFile, &oldSize)
	require.NoError(t, err)

	// Backdate to > 365 days
	_, err = pool.Exec(ctx,
		`UPDATE reports SET created_at = $1 WHERE id = $2`,
		time.Now().UTC().Add(-400*24*time.Hour), oldReportID)
	require.NoError(t, err)

	cleaner := engine.NewReportCleaner(reportStore, tmpDir, logger)
	err = cleaner.Cleanup(ctx)
	require.NoError(t, err)

	oldRptAfter, err := reportStore.GetByID(ctx, tenantID, oldReportID)
	require.NoError(t, err)
	assert.Equal(t, domain.ReportStatusCleaned, oldRptAfter.Status,
		"old report should be cleaned")

	t.Log("Full pipeline test passed: NATS -> ingestion -> " +
		"workflow -> escalation -> report -> cleanup")
}
