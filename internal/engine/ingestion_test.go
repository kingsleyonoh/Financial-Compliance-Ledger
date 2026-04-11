package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// testPoolDSN returns the pgx-compatible PostgreSQL connection string.
func testPoolDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

// newTestPool creates a pgxpool connection pool for testing.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testPoolDSN()

	err := store.RunMigrations(dsn, "file://../../migrations")
	require.NoError(t, err, "failed to run migrations")

	pool, err := store.NewPostgresPool(dsn)
	require.NoError(t, err, "failed to create test pool")

	t.Cleanup(func() { pool.Close() })
	return pool
}

// seedTenant inserts a test tenant and returns its UUID.
func seedTenant(t *testing.T, pool *pgxpool.Pool, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	apiKey := fmt.Sprintf("test-api-key-%s", id.String()[:8])
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, true)
	`, id, name, apiKey)
	require.NoError(t, err, "failed to seed tenant")
	return id
}

// cleanupTenantData deletes all data for the given tenant IDs.
func cleanupTenantData(t *testing.T, pool *pgxpool.Pool, tenantIDs ...uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, tid := range tenantIDs {
		_, _ = pool.Exec(ctx, `DELETE FROM ledger_events WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM escalation_rules WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM reports WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM discrepancies WHERE tenant_id = $1`, tid)
		_, _ = pool.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, tid)
	}
}

// testNATSURL returns the NATS server URL for testing.
func testNATSURL() string {
	url := os.Getenv("NATS_URL")
	if url == "" {
		url = "nats://localhost:4222"
	}
	return url
}

// newTestNATSConn creates a NATS connection for publishing test messages.
func newTestNATSConn(t *testing.T) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(testNATSURL())
	require.NoError(t, err, "failed to connect to NATS")
	t.Cleanup(func() { nc.Close() })
	return nc
}

// uniqueSubject generates a unique NATS subject for test isolation.
func uniqueSubject(prefix string) string {
	return fmt.Sprintf("test.%s.%s", prefix, uuid.New().String()[:8])
}

// ensureStream creates a JetStream stream for the test subject.
// Returns the stream name. Cleanup is automatic via t.Cleanup.
func ensureStream(t *testing.T, nc *nats.Conn, subject string) string {
	t.Helper()
	js, err := nc.JetStream()
	require.NoError(t, err, "failed to get JetStream context")

	streamName := "TEST_" + uuid.New().String()[:8]

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     streamName,
		Subjects: []string{subject},
		Storage:  nats.MemoryStorage,
	})
	require.NoError(t, err, "failed to create test stream")

	t.Cleanup(func() {
		_ = js.DeleteStream(streamName)
	})

	return streamName
}

// makeValidEnvelope creates a valid NATS message envelope for testing.
func makeValidEnvelope(tenantID uuid.UUID) map[string]interface{} {
	return map[string]interface{}{
		"event_type": "discrepancy.detected",
		"source":     "recon-engine",
		"tenant_id":  tenantID.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]interface{}{
			"external_id":      fmt.Sprintf("ext-%s", uuid.New().String()[:8]),
			"source_system":    "test-system",
			"discrepancy_type": "mismatch",
			"severity":         "high",
			"title":            "Test discrepancy from NATS",
			"description":      "Amount mismatch detected",
			"amount_expected":  1000.50,
			"amount_actual":    999.00,
			"currency":         "USD",
			"metadata":         map[string]interface{}{"ref": "TX-001"},
		},
	}
}

// publishJSON publishes a JSON message to the given subject via JetStream.
func publishJSON(t *testing.T, nc *nats.Conn, subject string, data interface{}) {
	t.Helper()
	js, err := nc.JetStream()
	require.NoError(t, err)

	bytes, err := json.Marshal(data)
	require.NoError(t, err)

	_, err = js.Publish(subject, bytes)
	require.NoError(t, err, "failed to publish message")
}

// testConfig returns a config suitable for tests with a unique consumer name.
func testConfig(subject, consumerName string) *config.Config {
	return &config.Config{
		NATSURL:          testNATSURL(),
		NATSSubject:      subject,
		NATSConsumerName: consumerName,
	}
}

// waitForCondition polls until condition returns true or timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func TestIngestion_ValidMessage_CreatesDiscrepancyAndEvent(t *testing.T) {
	pool := newTestPool(t)
	tenantID := seedTenant(t, pool, "ingestion-valid-tenant")
	defer cleanupTenantData(t, pool, tenantID)

	subject := uniqueSubject("valid")
	consumerName := fmt.Sprintf("test-valid-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err, "failed to create Ingestion")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err, "failed to start Ingestion")
	defer func() { _ = ing.Stop() }()

	// Publish a valid message
	envelope := makeValidEnvelope(tenantID)
	payload := envelope["payload"].(map[string]interface{})
	externalID := payload["external_id"].(string)
	publishJSON(t, nc, subject, envelope)

	// Wait for the discrepancy to appear
	waitForCondition(t, 5*time.Second, func() bool {
		d, err := discStore.GetByExternalID(
			context.Background(), tenantID, externalID,
		)
		return err == nil && d != nil
	})

	// Verify discrepancy was created
	d, err := discStore.GetByExternalID(
		context.Background(), tenantID, externalID,
	)
	require.NoError(t, err)
	assert.Equal(t, externalID, d.ExternalID)
	assert.Equal(t, "high", d.Severity)
	assert.Equal(t, "open", d.Status)
	assert.Equal(t, "mismatch", d.DiscrepancyType)
	assert.Equal(t, "Test discrepancy from NATS", d.Title)
	assert.Equal(t, "USD", d.Currency)

	// Verify ledger event was created
	discUUID, err := uuid.Parse(d.ID)
	require.NoError(t, err)
	events, err := eventStore.ListByDiscrepancy(
		context.Background(), tenantID, discUUID,
	)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "discrepancy.received", events[0].EventType)
	assert.Equal(t, "system", events[0].ActorType)
	assert.Equal(t, "nats-ingestion", events[0].Actor)
}

func TestIngestion_MalformedJSON_NaksMessage(t *testing.T) {
	pool := newTestPool(t)
	tenantID := seedTenant(t, pool, "ingestion-malformed-tenant")
	defer cleanupTenantData(t, pool, tenantID)

	subject := uniqueSubject("malformed")
	consumerName := fmt.Sprintf("test-malformed-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	// Publish malformed JSON
	js, err := nc.JetStream()
	require.NoError(t, err)
	_, err = js.Publish(subject, []byte("not-valid-json{{{"))
	require.NoError(t, err)

	// Wait for the message to be processed (NAK'd then dead-lettered)
	time.Sleep(1 * time.Second)

	discs, total, err := discStore.List(
		context.Background(), tenantID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, discs)
}

func TestIngestion_MissingRequiredFields_NaksMessage(t *testing.T) {
	pool := newTestPool(t)
	tenantID := seedTenant(t, pool, "ingestion-missing-fields-tenant")
	defer cleanupTenantData(t, pool, tenantID)

	subject := uniqueSubject("missingfields")
	consumerName := fmt.Sprintf("test-missing-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	// Publish message with missing tenant_id
	incomplete := map[string]interface{}{
		"event_type": "discrepancy.detected",
		"source":     "recon-engine",
		// "tenant_id" intentionally missing
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]interface{}{
			"external_id": "ext-missing-1",
		},
	}
	publishJSON(t, nc, subject, incomplete)

	// Wait for processing
	time.Sleep(1 * time.Second)

	discs, total, err := discStore.List(
		context.Background(), tenantID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, discs)
}

func TestIngestion_UnknownTenant_NaksMessage(t *testing.T) {
	pool := newTestPool(t)
	fakeTenantID := uuid.New()

	subject := uniqueSubject("unknowntenant")
	consumerName := fmt.Sprintf("test-unknown-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	envelope := makeValidEnvelope(fakeTenantID)
	publishJSON(t, nc, subject, envelope)

	// Wait for processing
	time.Sleep(1 * time.Second)

	discs, total, err := discStore.List(
		context.Background(), fakeTenantID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, discs)
}

func TestIngestion_InactiveTenant_NaksMessage(t *testing.T) {
	pool := newTestPool(t)
	inactiveID := uuid.New()
	apiKey := fmt.Sprintf("test-api-key-%s", inactiveID.String()[:8])
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO tenants (id, name, api_key, is_active)
		VALUES ($1, $2, $3, false)
	`, inactiveID, "inactive-tenant", apiKey)
	require.NoError(t, err)
	defer cleanupTenantData(t, pool, inactiveID)

	subject := uniqueSubject("inactive")
	consumerName := fmt.Sprintf("test-inactive-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctxCancel, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctxCancel)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	envelope := makeValidEnvelope(inactiveID)
	publishJSON(t, nc, subject, envelope)

	// Wait for processing
	time.Sleep(1 * time.Second)

	discs, total, err := discStore.List(
		context.Background(), inactiveID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, discs)
}

func TestIngestion_DuplicateExternalID_AcksWithoutCreating(t *testing.T) {
	pool := newTestPool(t)
	tenantID := seedTenant(t, pool, "ingestion-dedup-tenant")
	defer cleanupTenantData(t, pool, tenantID)

	subject := uniqueSubject("dedup")
	consumerName := fmt.Sprintf("test-dedup-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	// Publish first message
	envelope := makeValidEnvelope(tenantID)
	payload := envelope["payload"].(map[string]interface{})
	externalID := payload["external_id"].(string)
	publishJSON(t, nc, subject, envelope)

	// Wait for the first message to be processed
	waitForCondition(t, 5*time.Second, func() bool {
		d, err := discStore.GetByExternalID(
			context.Background(), tenantID, externalID,
		)
		return err == nil && d != nil
	})

	// Publish a duplicate with the same external_id
	publishJSON(t, nc, subject, envelope)

	// Wait for the duplicate to be processed
	time.Sleep(500 * time.Millisecond)

	// Should still only have 1 discrepancy
	discs, total, err := discStore.List(
		context.Background(), tenantID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, discs, 1)

	// Should still only have 1 ledger event
	discUUID, err := uuid.Parse(discs[0].ID)
	require.NoError(t, err)
	events, err := eventStore.ListByDiscrepancy(
		context.Background(), tenantID, discUUID,
	)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}

func TestIngestion_MissingPayloadFields_NaksMessage(t *testing.T) {
	pool := newTestPool(t)
	tenantID := seedTenant(t, pool, "ingestion-missing-payload-tenant")
	defer cleanupTenantData(t, pool, tenantID)

	subject := uniqueSubject("missingpayload")
	consumerName := fmt.Sprintf("test-mispay-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = ing.Stop() }()

	// Publish message with empty payload (missing required payload fields)
	envelope := map[string]interface{}{
		"event_type": "discrepancy.detected",
		"source":     "recon-engine",
		"tenant_id":  tenantID.String(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"payload":    map[string]interface{}{},
	}
	publishJSON(t, nc, subject, envelope)

	// Wait for processing
	time.Sleep(1 * time.Second)

	discs, total, err := discStore.List(
		context.Background(), tenantID, store.ListFilters{Limit: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, discs)
}

func TestIngestion_StopDrainsConnection(t *testing.T) {
	pool := newTestPool(t)

	subject := uniqueSubject("stop")
	consumerName := fmt.Sprintf("test-stop-%s", uuid.New().String()[:8])
	nc := newTestNATSConn(t)
	ensureStream(t, nc, subject)

	discStore := store.NewDiscrepancyStore(pool)
	eventStore := store.NewEventStore(pool)
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Logger()

	cfg := testConfig(subject, consumerName)
	ing, err := engine.NewIngestion(cfg, discStore, eventStore, pool, logger)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = ing.Start(ctx)
	require.NoError(t, err)

	// Stop should succeed without error
	err = ing.Stop()
	require.NoError(t, err)
}
