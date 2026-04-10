package store_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresConnectivity verifies the local PostgreSQL instance is reachable
// and can execute a simple query. Requires Docker services running
// (docker compose up -d).
func TestPostgresConnectivity(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger"
	}

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "failed to open database connection")
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var result int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	require.NoError(t, err, "failed to execute SELECT 1 query")
	assert.Equal(t, 1, result, "expected SELECT 1 to return 1")
}
