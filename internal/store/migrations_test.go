package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	store "github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// testDSN returns the PostgreSQL connection string for local dev.
func testDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

// migrationsPath returns the absolute file:// path to the migrations directory.
func migrationsPath() string {
	// The test binary runs from the package directory (internal/store/).
	// Migrations live at the repo root: ../../migrations
	return "file://../../migrations"
}

// openDB creates a database connection for testing.
func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", testDSN())
	require.NoError(t, err, "failed to open database connection")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx), "failed to ping database")
	return db
}

// newMigrate creates a migrate instance pointing at the local PostgreSQL.
func newMigrate(t *testing.T, db *sql.DB) *migrate.Migrate {
	t.Helper()
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	require.NoError(t, err, "failed to create postgres driver for migrate")

	m, err := migrate.NewWithDatabaseInstance(migrationsPath(), "postgres", driver)
	require.NoError(t, err, "failed to create migrate instance")
	return m
}

// tableExists checks whether a table exists in the public schema.
func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, tableName).Scan(&exists)
	require.NoError(t, err, "failed to check table existence for %s", tableName)
	return exists
}

// columnExists checks whether a column exists in a table in the public schema.
func columnExists(t *testing.T, db *sql.DB, tableName, columnName string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND column_name = $2
		)
	`, tableName, columnName).Scan(&exists)
	require.NoError(t, err, "failed to check column existence for %s.%s", tableName, columnName)
	return exists
}

// indexExists checks whether an index exists.
func indexExists(t *testing.T, db *sql.DB, indexName string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'public' AND indexname = $1
		)
	`, indexName).Scan(&exists)
	require.NoError(t, err, "failed to check index existence for %s", indexName)
	return exists
}

// columnDataType returns the data_type for a given table.column.
func columnDataType(t *testing.T, db *sql.DB, tableName, columnName string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dt string
	err := db.QueryRowContext(ctx, `
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = $1
		  AND column_name = $2
	`, tableName, columnName).Scan(&dt)
	require.NoError(t, err, "failed to get data_type for %s.%s", tableName, columnName)
	return dt
}

// constraintExists checks whether a named constraint exists.
func constraintExists(t *testing.T, db *sql.DB, tableName, constraintName string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.table_constraints
			WHERE table_schema = 'public'
			  AND table_name = $1
			  AND constraint_name = $2
		)
	`, tableName, constraintName).Scan(&exists)
	require.NoError(t, err, "failed to check constraint %s on %s", constraintName, tableName)
	return exists
}

// ---------- Tests ----------

// TestMigrationsUpAndDown runs all up migrations, verifies schema, then
// runs all down migrations and verifies tables are dropped.
func TestMigrationsUpAndDown(t *testing.T) {
	db := openDB(t)
	defer db.Close()

	// Ensure a clean state: drop all tables that might linger from
	// a previous interrupted run.
	cleanDB(t, db)

	m := newMigrate(t, db)

	// --- UP ---
	err := m.Up()
	require.NoError(t, err, "failed to run up migrations")

	// Verify all six tables exist.
	tables := []string{
		"tenants",
		"discrepancies",
		"ledger_events",
		"escalation_rules",
		"reports",
		"notification_log",
	}
	for _, tbl := range tables {
		assert.True(t, tableExists(t, db, tbl), "table %s should exist after up migrations", tbl)
	}

	// --- DOWN ---
	err = m.Down()
	require.NoError(t, err, "failed to run down migrations")

	for _, tbl := range tables {
		assert.False(t, tableExists(t, db, tbl), "table %s should NOT exist after down migrations", tbl)
	}
}

// TestTenantsTableSchema verifies 001_create_tenants columns and indexes.
func TestTenantsTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	// Columns
	cols := []string{"id", "name", "api_key", "is_active", "settings", "created_at", "updated_at"}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "tenants", col), "tenants.%s should exist", col)
	}

	// Column types
	assert.Equal(t, "uuid", columnDataType(t, db, "tenants", "id"))
	assert.Equal(t, "character varying", columnDataType(t, db, "tenants", "name"))
	assert.Equal(t, "boolean", columnDataType(t, db, "tenants", "is_active"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "tenants", "settings"))

	// Indexes
	assert.True(t, indexExists(t, db, "idx_tenants_api_key"), "idx_tenants_api_key should exist")
	assert.True(t, indexExists(t, db, "idx_tenants_is_active"), "idx_tenants_is_active should exist")
}

// TestDiscrepanciesTableSchema verifies 002_create_discrepancies columns and indexes.
func TestDiscrepanciesTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	cols := []string{
		"id", "tenant_id", "external_id", "source_system", "discrepancy_type",
		"severity", "status", "title", "description", "amount_expected",
		"amount_actual", "currency", "metadata", "first_detected_at",
		"resolved_at", "created_at", "updated_at",
	}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "discrepancies", col), "discrepancies.%s should exist", col)
	}

	// Column types
	assert.Equal(t, "uuid", columnDataType(t, db, "discrepancies", "id"))
	assert.Equal(t, "uuid", columnDataType(t, db, "discrepancies", "tenant_id"))
	assert.Equal(t, "numeric", columnDataType(t, db, "discrepancies", "amount_expected"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "discrepancies", "metadata"))

	// Indexes
	assert.True(t, indexExists(t, db, "idx_discrepancies_tenant_id"), "idx_discrepancies_tenant_id should exist")
	assert.True(t, indexExists(t, db, "idx_discrepancies_status"), "idx_discrepancies_status should exist")
	assert.True(t, indexExists(t, db, "idx_discrepancies_severity"), "idx_discrepancies_severity should exist")
	assert.True(t, indexExists(t, db, "idx_discrepancies_created_at"), "idx_discrepancies_created_at should exist")
	assert.True(t, indexExists(t, db, "idx_discrepancies_type"), "idx_discrepancies_type should exist")
}

// TestLedgerEventsTableSchema verifies 003_create_ledger_events columns and indexes.
func TestLedgerEventsTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	cols := []string{
		"id", "tenant_id", "discrepancy_id", "event_type", "actor",
		"actor_type", "payload", "sequence_num", "created_at",
	}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "ledger_events", col), "ledger_events.%s should exist", col)
	}

	assert.Equal(t, "uuid", columnDataType(t, db, "ledger_events", "id"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "ledger_events", "payload"))

	assert.True(t, indexExists(t, db, "idx_ledger_events_discrepancy"), "idx_ledger_events_discrepancy should exist")
	assert.True(t, indexExists(t, db, "idx_ledger_events_tenant"), "idx_ledger_events_tenant should exist")
	assert.True(t, indexExists(t, db, "idx_ledger_events_type"), "idx_ledger_events_type should exist")
}

// TestEscalationRulesTableSchema verifies 004_create_escalation_rules columns and indexes.
func TestEscalationRulesTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	cols := []string{
		"id", "tenant_id", "name", "severity_match", "trigger_after_hrs",
		"trigger_status", "action", "action_config", "is_active",
		"priority", "created_at", "updated_at",
	}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "escalation_rules", col), "escalation_rules.%s should exist", col)
	}

	assert.Equal(t, "uuid", columnDataType(t, db, "escalation_rules", "id"))
	assert.Equal(t, "integer", columnDataType(t, db, "escalation_rules", "trigger_after_hrs"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "escalation_rules", "action_config"))
	assert.Equal(t, "boolean", columnDataType(t, db, "escalation_rules", "is_active"))

	assert.True(t, indexExists(t, db, "idx_escalation_rules_tenant"), "idx_escalation_rules_tenant should exist")
}

// TestReportsTableSchema verifies 005_create_reports columns and indexes.
func TestReportsTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	cols := []string{
		"id", "tenant_id", "report_type", "title", "parameters",
		"status", "file_path", "file_size_bytes", "generated_by",
		"created_at", "updated_at",
	}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "reports", col), "reports.%s should exist", col)
	}

	assert.Equal(t, "uuid", columnDataType(t, db, "reports", "id"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "reports", "parameters"))
	assert.Equal(t, "bigint", columnDataType(t, db, "reports", "file_size_bytes"))

	assert.True(t, indexExists(t, db, "idx_reports_tenant"), "idx_reports_tenant should exist")
}

// TestDiscrepancyCheckConstraints verifies CHECK constraints on discrepancies
// by attempting to insert invalid values.
func TestDiscrepancyCheckConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert a tenant first (FK dependency).
	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_1')
	`)
	require.NoError(t, err, "failed to insert test tenant")

	// Valid insert should succeed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (
			tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title
		) VALUES (
			'a0000000-0000-0000-0000-000000000001', 'EXT-001', 'test-system',
			'missing', 'high', 'open', 'Test discrepancy'
		)
	`)
	assert.NoError(t, err, "valid insert should succeed")

	// Invalid discrepancy_type should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (
			tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title
		) VALUES (
			'a0000000-0000-0000-0000-000000000001', 'EXT-002', 'test-system',
			'invalid_type', 'high', 'open', 'Bad type'
		)
	`)
	assert.Error(t, err, "invalid discrepancy_type should be rejected by CHECK constraint")

	// Invalid severity should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (
			tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title
		) VALUES (
			'a0000000-0000-0000-0000-000000000001', 'EXT-003', 'test-system',
			'missing', 'extreme', 'open', 'Bad severity'
		)
	`)
	assert.Error(t, err, "invalid severity should be rejected by CHECK constraint")

	// Invalid status should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (
			tenant_id, external_id, source_system, discrepancy_type,
			severity, status, title
		) VALUES (
			'a0000000-0000-0000-0000-000000000001', 'EXT-004', 'test-system',
			'missing', 'high', 'invalid_status', 'Bad status'
		)
	`)
	assert.Error(t, err, "invalid status should be rejected by CHECK constraint")
}

// TestLedgerEventsCheckConstraints verifies CHECK constraints on ledger_events.
func TestLedgerEventsCheckConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert tenant and discrepancy for FK.
	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_1')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (id, tenant_id, external_id, source_system, discrepancy_type, severity, status, title)
		VALUES ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'EXT-100', 'test', 'missing', 'high', 'open', 'Test')
	`)
	require.NoError(t, err)

	// Valid event_type should succeed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO ledger_events (tenant_id, discrepancy_id, event_type, actor)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'discrepancy.received', 'system')
	`)
	assert.NoError(t, err, "valid event_type should succeed")

	// Invalid event_type should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO ledger_events (tenant_id, discrepancy_id, event_type, actor)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'invalid.type', 'system')
	`)
	assert.Error(t, err, "invalid event_type should be rejected by CHECK constraint")

	// Invalid actor_type should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO ledger_events (tenant_id, discrepancy_id, event_type, actor, actor_type)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'discrepancy.received', 'bot', 'robot')
	`)
	assert.Error(t, err, "invalid actor_type should be rejected by CHECK constraint")
}

// TestEscalationRulesCheckConstraints verifies CHECK constraints on escalation_rules.
func TestEscalationRulesCheckConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_1')
	`)
	require.NoError(t, err)

	// Valid rule should succeed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Auto-close low after 72h', 'low', 72, 'auto_close')
	`)
	assert.NoError(t, err, "valid escalation rule should succeed")

	// Invalid severity_match should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Bad rule', 'extreme', 24, 'notify')
	`)
	assert.Error(t, err, "invalid severity_match should be rejected")

	// Invalid action should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Bad action rule', 'high', 24, 'delete')
	`)
	assert.Error(t, err, "invalid action should be rejected")
}

// TestReportsCheckConstraints verifies CHECK constraints on reports.
func TestReportsCheckConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_1')
	`)
	require.NoError(t, err)

	// Valid report should succeed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO reports (tenant_id, report_type, status)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'daily_summary', 'pending')
	`)
	assert.NoError(t, err, "valid report should succeed")

	// Invalid report_type should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO reports (tenant_id, report_type, status)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'invalid_type', 'pending')
	`)
	assert.Error(t, err, "invalid report_type should be rejected")

	// Invalid status should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO reports (tenant_id, report_type, status)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'daily_summary', 'cancelled')
	`)
	assert.Error(t, err, "invalid report status should be rejected")
}

// TestUniqueConstraints verifies unique constraints across tables.
func TestUniqueConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// --- tenants: api_key must be unique ---
	_, err := db.ExecContext(ctx, `INSERT INTO tenants (name, api_key) VALUES ('Tenant A', 'same_key')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO tenants (name, api_key) VALUES ('Tenant B', 'same_key')`)
	assert.Error(t, err, "duplicate api_key should be rejected")

	// --- discrepancies: (tenant_id, external_id) must be unique ---
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (tenant_id, external_id, source_system, discrepancy_type, severity, title)
		VALUES ((SELECT id FROM tenants WHERE name='Tenant A'), 'DUP-001', 'sys', 'missing', 'low', 'First')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (tenant_id, external_id, source_system, discrepancy_type, severity, title)
		VALUES ((SELECT id FROM tenants WHERE name='Tenant A'), 'DUP-001', 'sys', 'mismatch', 'high', 'Duplicate')
	`)
	assert.Error(t, err, "duplicate (tenant_id, external_id) should be rejected")

	// --- escalation_rules: (tenant_id, name) must be unique ---
	_, err = db.ExecContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ((SELECT id FROM tenants WHERE name='Tenant A'), 'Rule One', 'high', 24, 'notify')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ((SELECT id FROM tenants WHERE name='Tenant A'), 'Rule One', 'low', 48, 'escalate')
	`)
	assert.Error(t, err, "duplicate (tenant_id, name) should be rejected for escalation_rules")
}

// TestForeignKeyConstraints verifies FK relationships enforce referential integrity.
func TestForeignKeyConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nonExistentTenantID := "00000000-0000-0000-0000-000000000099"

	// Discrepancy with non-existent tenant_id should fail.
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO discrepancies (tenant_id, external_id, source_system, discrepancy_type, severity, title)
		VALUES ('%s', 'EXT-FK-1', 'sys', 'missing', 'low', 'Orphan')
	`, nonExistentTenantID))
	assert.Error(t, err, "discrepancy with non-existent tenant_id should be rejected by FK")

	// Ledger event with non-existent discrepancy_id should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'FK Tenant', 'fk_key')
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO ledger_events (tenant_id, discrepancy_id, event_type, actor)
		VALUES ('a0000000-0000-0000-0000-000000000001', '%s', 'discrepancy.received', 'system')
	`, "00000000-0000-0000-0000-000000000099"))
	assert.Error(t, err, "ledger_event with non-existent discrepancy_id should be rejected by FK")

	// Escalation rule with non-existent tenant_id should fail.
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ('%s', 'Orphan Rule', 'high', 24, 'notify')
	`, nonExistentTenantID))
	assert.Error(t, err, "escalation_rule with non-existent tenant_id should be rejected by FK")

	// Report with non-existent tenant_id should fail.
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO reports (tenant_id, report_type)
		VALUES ('%s', 'daily_summary')
	`, nonExistentTenantID))
	assert.Error(t, err, "report with non-existent tenant_id should be rejected by FK")
}

// TestDefaultValues verifies that default values are applied correctly.
func TestDefaultValues(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert tenant with minimal fields.
	var isActive bool
	var settings string
	err := db.QueryRowContext(ctx, `
		INSERT INTO tenants (name, api_key) VALUES ('Default Tenant', 'default_key')
		RETURNING is_active, settings::text
	`).Scan(&isActive, &settings)
	require.NoError(t, err)
	assert.True(t, isActive, "is_active should default to true")
	assert.Equal(t, "{}", settings, "settings should default to empty JSON object")

	// Insert discrepancy with minimal fields — status should default to 'open'.
	var tenantID string
	err = db.QueryRowContext(ctx, `SELECT id FROM tenants WHERE name='Default Tenant'`).Scan(&tenantID)
	require.NoError(t, err)

	var status, currency string
	err = db.QueryRowContext(ctx, `
		INSERT INTO discrepancies (tenant_id, external_id, source_system, discrepancy_type, severity, title)
		VALUES ($1, 'DEF-001', 'sys', 'missing', 'low', 'Default test')
		RETURNING status, currency
	`, tenantID).Scan(&status, &currency)
	require.NoError(t, err)
	assert.Equal(t, "open", status, "status should default to 'open'")
	assert.Equal(t, "USD", currency, "currency should default to 'USD'")

	// Insert ledger event — actor_type should default to 'system'.
	var discID string
	err = db.QueryRowContext(ctx, `SELECT id FROM discrepancies WHERE external_id='DEF-001'`).Scan(&discID)
	require.NoError(t, err)

	var actorType string
	err = db.QueryRowContext(ctx, `
		INSERT INTO ledger_events (tenant_id, discrepancy_id, event_type, actor)
		VALUES ($1, $2, 'discrepancy.received', 'system')
		RETURNING actor_type
	`, tenantID, discID).Scan(&actorType)
	require.NoError(t, err)
	assert.Equal(t, "system", actorType, "actor_type should default to 'system'")

	// Escalation rule defaults.
	var triggerStatus string
	var isRuleActive bool
	var priority int
	err = db.QueryRowContext(ctx, `
		INSERT INTO escalation_rules (tenant_id, name, severity_match, trigger_after_hrs, action)
		VALUES ($1, 'Default Rule', 'high', 24, 'notify')
		RETURNING trigger_status, is_active, priority
	`, tenantID).Scan(&triggerStatus, &isRuleActive, &priority)
	require.NoError(t, err)
	assert.Equal(t, "open", triggerStatus, "trigger_status should default to 'open'")
	assert.True(t, isRuleActive, "is_active should default to true")
	assert.Equal(t, 0, priority, "priority should default to 0")

	// Report defaults.
	var reportStatus string
	err = db.QueryRowContext(ctx, `
		INSERT INTO reports (tenant_id, report_type)
		VALUES ($1, 'daily_summary')
		RETURNING status
	`, tenantID).Scan(&reportStatus)
	require.NoError(t, err)
	assert.Equal(t, "pending", reportStatus, "report status should default to 'pending'")
}

// TestNotificationLogTableSchema verifies 006_create_notification_log columns and indexes.
func TestNotificationLogTableSchema(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	cols := []string{
		"id", "tenant_id", "discrepancy_id", "escalation_rule_id",
		"channel", "recipient", "status", "hub_response",
		"attempts", "last_attempt_at", "created_at", "updated_at",
	}
	for _, col := range cols {
		assert.True(t, columnExists(t, db, "notification_log", col), "notification_log.%s should exist", col)
	}

	// Column types
	assert.Equal(t, "uuid", columnDataType(t, db, "notification_log", "id"))
	assert.Equal(t, "uuid", columnDataType(t, db, "notification_log", "tenant_id"))
	assert.Equal(t, "uuid", columnDataType(t, db, "notification_log", "discrepancy_id"))
	assert.Equal(t, "character varying", columnDataType(t, db, "notification_log", "channel"))
	assert.Equal(t, "character varying", columnDataType(t, db, "notification_log", "recipient"))
	assert.Equal(t, "character varying", columnDataType(t, db, "notification_log", "status"))
	assert.Equal(t, "jsonb", columnDataType(t, db, "notification_log", "hub_response"))
	assert.Equal(t, "integer", columnDataType(t, db, "notification_log", "attempts"))

	// Indexes
	assert.True(t, indexExists(t, db, "idx_notification_log_tenant"), "idx_notification_log_tenant should exist")
	assert.True(t, indexExists(t, db, "idx_notification_log_status"), "idx_notification_log_status should exist")
	assert.True(t, indexExists(t, db, "idx_notification_log_discrepancy"), "idx_notification_log_discrepancy should exist")
}

// TestNotificationLogCheckConstraints verifies CHECK constraints on notification_log.
func TestNotificationLogCheckConstraints(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Insert tenant and discrepancy for FK.
	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_notif')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (id, tenant_id, external_id, source_system, discrepancy_type, severity, status, title)
		VALUES ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'EXT-NOTIF-1', 'test', 'missing', 'high', 'open', 'Test')
	`)
	require.NoError(t, err)

	// Valid notification should succeed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'email', 'user@example.com')
	`)
	assert.NoError(t, err, "valid notification_log insert should succeed")

	// Invalid channel should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'sms', 'user@example.com')
	`)
	assert.Error(t, err, "invalid channel should be rejected by CHECK constraint")

	// Invalid status should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient, status)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'webhook', 'https://hook.example.com', 'delivered')
	`)
	assert.Error(t, err, "invalid status should be rejected by CHECK constraint")
}

// TestNotificationLogDefaults verifies default values on notification_log.
func TestNotificationLogDefaults(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_notif2')
	`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (id, tenant_id, external_id, source_system, discrepancy_type, severity, status, title)
		VALUES ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'EXT-NOTIF-DEF', 'test', 'missing', 'high', 'open', 'Test')
	`)
	require.NoError(t, err)

	var status string
	var attempts int
	err = db.QueryRowContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', 'email', 'user@example.com')
		RETURNING status, attempts
	`).Scan(&status, &attempts)
	require.NoError(t, err)
	assert.Equal(t, "pending", status, "status should default to 'pending'")
	assert.Equal(t, 0, attempts, "attempts should default to 0")
}

// TestNotificationLogForeignKeys verifies FK relationships on notification_log.
func TestNotificationLogForeignKeys(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)
	m := newMigrate(t, db)
	require.NoError(t, m.Up(), "up migrations failed")
	defer func() { _ = m.Down() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Non-existent tenant_id should fail.
	_, err := db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient)
		VALUES ('00000000-0000-0000-0000-000000000099', '00000000-0000-0000-0000-000000000099', 'email', 'user@example.com')
	`)
	assert.Error(t, err, "notification_log with non-existent tenant_id should be rejected by FK")

	// Insert tenant but non-existent discrepancy_id should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO tenants (id, name, api_key)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'Test Tenant', 'hashed_key_notif3')
	`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000099', 'email', 'user@example.com')
	`)
	assert.Error(t, err, "notification_log with non-existent discrepancy_id should be rejected by FK")

	// Nullable escalation_rule_id: null should succeed, non-existent should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO discrepancies (id, tenant_id, external_id, source_system, discrepancy_type, severity, status, title)
		VALUES ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'EXT-FK-N', 'test', 'missing', 'high', 'open', 'Test')
	`)
	require.NoError(t, err)

	// Null escalation_rule_id is allowed.
	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, escalation_rule_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', NULL, 'email', 'user@example.com')
	`)
	assert.NoError(t, err, "null escalation_rule_id should be allowed")

	// Non-existent escalation_rule_id should fail.
	_, err = db.ExecContext(ctx, `
		INSERT INTO notification_log (tenant_id, discrepancy_id, escalation_rule_id, channel, recipient)
		VALUES ('a0000000-0000-0000-0000-000000000001', 'b0000000-0000-0000-0000-000000000001', '00000000-0000-0000-0000-000000000099', 'email', 'user@example.com')
	`)
	assert.Error(t, err, "notification_log with non-existent escalation_rule_id should be rejected by FK")
}

// TestNewPostgresPool tests the NewPostgresPool function from postgres.go.
func TestNewPostgresPool(t *testing.T) {
	pool, err := store.NewPostgresPool(testDSN())
	require.NoError(t, err, "NewPostgresPool should not return an error")
	require.NotNil(t, pool, "pool should not be nil")
	defer pool.Close()

	// Verify the pool can execute a query.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err, "pool should execute queries")
	assert.Equal(t, 1, result, "expected SELECT 1 to return 1")
}

// TestNewPostgresPoolInvalidURL tests that an invalid URL returns an error.
func TestNewPostgresPoolInvalidURL(t *testing.T) {
	pool, err := store.NewPostgresPool("postgres://invalid:invalid@localhost:9999/nonexistent?sslmode=disable&connect_timeout=2")
	assert.Error(t, err, "NewPostgresPool should return an error for invalid URL")
	assert.Nil(t, pool, "pool should be nil on error")
}

// TestRunMigrations tests the RunMigrations function from postgres.go.
func TestRunMigrations(t *testing.T) {
	db := openDB(t)
	defer db.Close()
	cleanDB(t, db)

	err := store.RunMigrations(testDSN(), migrationsPath())
	require.NoError(t, err, "RunMigrations should not return an error")

	// Re-open to verify tables after migration.
	db2 := openDB(t)
	defer db2.Close()

	// Verify all tables exist after RunMigrations.
	tables := []string{
		"tenants",
		"discrepancies",
		"ledger_events",
		"escalation_rules",
		"reports",
		"notification_log",
	}
	for _, tbl := range tables {
		assert.True(t, tableExists(t, db2, tbl), "table %s should exist after RunMigrations", tbl)
	}

	// Clean up.
	cleanDB(t, db2)
}

// TestRunMigrationsInvalidURL tests that RunMigrations with a bad URL returns an error.
func TestRunMigrationsInvalidURL(t *testing.T) {
	err := store.RunMigrations("postgres://invalid:invalid@localhost:9999/nonexistent?sslmode=disable&connect_timeout=2", migrationsPath())
	assert.Error(t, err, "RunMigrations should return an error for invalid URL")
}

// cleanDB drops all six migration tables if they exist.
func cleanDB(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Drop in reverse order of dependencies.
	tables := []string{"notification_log", "reports", "escalation_rules", "ledger_events", "discrepancies", "tenants"}
	for _, tbl := range tables {
		_, err := db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tbl))
		require.NoError(t, err, "failed to drop table %s during cleanup", tbl)
	}

	// Also clean up the migrate schema_migrations table.
	_, _ = db.ExecContext(ctx, "DROP TABLE IF EXISTS schema_migrations CASCADE")
}
