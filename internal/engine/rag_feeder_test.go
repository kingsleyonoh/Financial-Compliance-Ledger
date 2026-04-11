package engine_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func TestRAGFeeder_FeedResolved_Disabled(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	cfg := &config.Config{
		RAGFeedEnabled: false,
	}
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	disc := &domain.Discrepancy{
		ID:       uuid.New().String(),
		TenantID: uuid.New().String(),
		Title:    "Test",
		Status:   domain.StatusResolved,
	}

	err := feeder.FeedResolved(context.Background(), uuid.New(), disc)
	assert.NoError(t, err, "disabled feeder should return nil")
}

func TestRAGFeeder_FeedResolved_Success(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "rag-feed-ok")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	// Create a discrepancy
	disc := &domain.Discrepancy{
		ExternalID:      "ext-rag-001",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityHigh,
		Status:          domain.StatusResolved,
		Title:           "Mismatch in Q1 report",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(context.Background(), tenantID, disc)
	require.NoError(t, err)

	discID, _ := uuid.Parse(created.ID)

	// Add events for timeline
	_, err = es.Append(context.Background(), tenantID, &domain.LedgerEvent{
		DiscrepancyID: discID.String(),
		EventType:     domain.EventReceived,
		Actor:         "system",
		ActorType:     domain.ActorSystem,
		Payload:       map[string]interface{}{"note": "received"},
	})
	require.NoError(t, err)

	_, err = es.Append(context.Background(), tenantID, &domain.LedgerEvent{
		DiscrepancyID: discID.String(),
		EventType:     domain.EventResolved,
		Actor:         "analyst",
		ActorType:     domain.ActorUser,
		Payload: map[string]interface{}{
			"resolution_type": "match_found",
		},
	})
	require.NoError(t, err)

	// Mock RAG Platform server
	var receivedBody string
	var receivedContentType string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		RAGFeedEnabled:    true,
		RAGPlatformURL:    mockServer.URL,
		RAGPlatformAPIKey: "test-rag-key",
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	err = feeder.FeedResolved(context.Background(), tenantID, created)
	require.NoError(t, err)

	// Verify the request was made with multipart content
	assert.True(t, strings.Contains(receivedContentType, "multipart/form-data"),
		"request should be multipart/form-data, got: %s", receivedContentType)
	assert.NotEmpty(t, receivedBody)
}

func TestRAGFeeder_FeedResolved_ServerUnreachable_ReturnsNil(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "rag-unreach")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	disc := &domain.Discrepancy{
		ExternalID:      "ext-rag-unreach",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityLow,
		Status:          domain.StatusResolved,
		Title:           "Unreachable test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(context.Background(), tenantID, disc)
	require.NoError(t, err)

	cfg := &config.Config{
		RAGFeedEnabled:    true,
		RAGPlatformURL:    "http://127.0.0.1:19999", // unreachable
		RAGPlatformAPIKey: "test-key",
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	err = feeder.FeedResolved(context.Background(), tenantID, created)
	assert.NoError(t, err, "unreachable RAG Platform should return nil (non-critical)")
}

func TestRAGFeeder_FeedResolved_TruncatesLargeTimeline(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "rag-truncate")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	disc := &domain.Discrepancy{
		ExternalID:      "ext-rag-trunc",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityMedium,
		Status:          domain.StatusResolved,
		Title:           "Truncation test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(context.Background(), tenantID, disc)
	require.NoError(t, err)

	discID, _ := uuid.Parse(created.ID)

	// Insert 55 events (more than 50 limit)
	for i := 0; i < 55; i++ {
		_, err = es.Append(context.Background(), tenantID, &domain.LedgerEvent{
			DiscrepancyID: discID.String(),
			EventType:     domain.EventNoteAdded,
			Actor:         "system",
			ActorType:     domain.ActorSystem,
			Payload:       map[string]interface{}{"idx": i},
		})
		require.NoError(t, err)
	}

	var eventCount int
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// Count timeline entries in the markdown
		eventCount = strings.Count(string(body), "- discrepancy.")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		RAGFeedEnabled:    true,
		RAGPlatformURL:    mockServer.URL,
		RAGPlatformAPIKey: "test-key",
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	err = feeder.FeedResolved(context.Background(), tenantID, created)
	require.NoError(t, err)

	assert.LessOrEqual(t, eventCount, 50,
		"timeline should be truncated to at most 50 events")
}

func TestRAGFeeder_FeedResolved_APIKeyHeader(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "rag-apikey")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	disc := &domain.Discrepancy{
		ExternalID:      "ext-rag-key",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMissing,
		Severity:        domain.SeverityLow,
		Status:          domain.StatusResolved,
		Title:           "API key test",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(context.Background(), tenantID, disc)
	require.NoError(t, err)

	var receivedKey string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		RAGFeedEnabled:    true,
		RAGPlatformURL:    mockServer.URL,
		RAGPlatformAPIKey: "my-secret-rag-key",
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	err = feeder.FeedResolved(context.Background(), tenantID, created)
	require.NoError(t, err)

	assert.Equal(t, "my-secret-rag-key", receivedKey,
		"X-API-Key header should be set from config")
}

func TestRAGFeeder_FeedResolved_MarkdownFormat(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "rag-format")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	disc := &domain.Discrepancy{
		ExternalID:      "ext-rag-fmt",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityCritical,
		Status:          domain.StatusResolved,
		Title:           "Format Verification",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	created, err := ds.Create(context.Background(), tenantID, disc)
	require.NoError(t, err)

	discID, _ := uuid.Parse(created.ID)
	_, err = es.Append(context.Background(), tenantID, &domain.LedgerEvent{
		DiscrepancyID: discID.String(),
		EventType:     domain.EventResolved,
		Actor:         "analyst",
		ActorType:     domain.ActorUser,
		Payload:       map[string]interface{}{"resolution_type": "false_positive"},
	})
	require.NoError(t, err)

	var receivedBody string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedBody = string(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		RAGFeedEnabled:    true,
		RAGPlatformURL:    mockServer.URL,
		RAGPlatformAPIKey: "test-key",
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)

	err = feeder.FeedResolved(context.Background(), tenantID, created)
	require.NoError(t, err)

	// Verify YAML frontmatter markers present
	assert.Contains(t, receivedBody, "---")
	assert.Contains(t, receivedBody, "severity: critical")
	assert.Contains(t, receivedBody, "# Discrepancy: Format Verification")
	assert.Contains(t, receivedBody, "## Timeline")
}

// ---------- RAG Sync Background Job Tests ----------

func TestRAGSyncer_StartSync_StopsOnCancel(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)
	es := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	cfg := &config.Config{
		RAGFeedEnabled: false,
	}

	feeder := engine.NewRAGFeeder(cfg, ds, es, logger)
	syncer := engine.NewRAGSyncer(feeder, ds, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		syncer.StartSync(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("StartSync did not stop after context cancellation")
	}
}

// ---------- ListAllResolvedSince store test ----------

func TestDiscrepancyStore_ListAllResolvedSince(t *testing.T) {
	pool := newTestPool(t)
	ds := store.NewDiscrepancyStore(pool)

	tenantA := seedTenant(t, pool, "rag-resolved-a")
	tenantB := seedTenant(t, pool, "rag-resolved-b")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantA, tenantB) })

	ctx := context.Background()
	now := time.Now().UTC()
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Seed resolved discrepancy for tenant A
	discA := &domain.Discrepancy{
		ExternalID:      "ext-resolve-a",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMismatch,
		Severity:        domain.SeverityHigh,
		Status:          domain.StatusResolved,
		Title:           "Resolved A",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	_, err := ds.Create(ctx, tenantA, discA)
	require.NoError(t, err)

	// Set resolved_at for tenant A's discrepancy
	resolvedAt := now.Add(-1 * time.Hour)
	_, err = pool.Exec(ctx, `
		UPDATE discrepancies SET resolved_at = $1, status = 'resolved'
		WHERE tenant_id = $2 AND external_id = 'ext-resolve-a'
	`, resolvedAt, tenantA)
	require.NoError(t, err)

	// Seed auto_closed discrepancy for tenant B
	discB := &domain.Discrepancy{
		ExternalID:      "ext-resolve-b",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeDuplicate,
		Severity:        domain.SeverityLow,
		Status:          domain.StatusAutoClosed,
		Title:           "Auto-Closed B",
		Currency:        "EUR",
		Metadata:        map[string]interface{}{},
	}
	_, err = ds.Create(ctx, tenantB, discB)
	require.NoError(t, err)

	resolvedAtB := now.Add(-30 * time.Minute)
	_, err = pool.Exec(ctx, `
		UPDATE discrepancies SET resolved_at = $1, status = 'auto_closed'
		WHERE tenant_id = $2 AND external_id = 'ext-resolve-b'
	`, resolvedAtB, tenantB)
	require.NoError(t, err)

	// Seed open discrepancy (should not be returned)
	discOpen := &domain.Discrepancy{
		ExternalID:      "ext-open-c",
		SourceSystem:    "test-system",
		DiscrepancyType: domain.TypeMissing,
		Severity:        domain.SeverityMedium,
		Status:          domain.StatusOpen,
		Title:           "Open C",
		Currency:        "USD",
		Metadata:        map[string]interface{}{},
	}
	_, err = ds.Create(ctx, tenantA, discOpen)
	require.NoError(t, err)

	results, err := ds.ListAllResolvedSince(ctx, twoHoursAgo)
	require.NoError(t, err)

	// Should find both resolved and auto_closed, but not open
	assert.GreaterOrEqual(t, len(results), 2,
		"should return resolved and auto_closed discrepancies")

	var foundA, foundB bool
	for _, d := range results {
		if d.ExternalID == "ext-resolve-a" {
			foundA = true
		}
		if d.ExternalID == "ext-resolve-b" {
			foundB = true
		}
		assert.NotEqual(t, "ext-open-c", d.ExternalID,
			"open discrepancy should not appear")
	}
	assert.True(t, foundA, "tenant A's resolved should be found")
	assert.True(t, foundB, "tenant B's auto_closed should be found")
}
