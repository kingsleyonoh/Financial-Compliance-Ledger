package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// testPoolDSN returns the PostgreSQL connection string for tests.
func testPoolDSN() string {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://fcl:localdev@localhost:5441/compliance_ledger?sslmode=disable"
	}
	return dsn
}

// newTestPool creates a pgxpool for testing with migrations applied.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testPoolDSN()

	err := store.RunMigrations(dsn, "file://../../../migrations")
	require.NoError(t, err, "failed to run migrations")

	pool, err := store.NewPostgresPool(dsn)
	require.NoError(t, err, "failed to create test pool")

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func TestHealthHandler_Handle_ReturnsOKWithConnectedDB(t *testing.T) {
	pool := newTestPool(t)

	h := handlers.NewHealthHandler(pool, "nats://localhost:4222")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "connected", body["pg"])
}

func TestHealthHandler_Handle_ResponseFormat(t *testing.T) {
	pool := newTestPool(t)

	h := handlers.NewHealthHandler(pool, "nats://localhost:4222")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	// Must contain exactly these three keys
	_, hasStatus := body["status"]
	_, hasPG := body["pg"]
	_, hasNATS := body["nats"]
	assert.True(t, hasStatus, "response must contain 'status'")
	assert.True(t, hasPG, "response must contain 'pg'")
	assert.True(t, hasNATS, "response must contain 'nats'")
}

func TestHealthHandler_Handle_NilPoolReturnsDisconnected(t *testing.T) {
	h := handlers.NewHealthHandler(nil, "nats://localhost:4222")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	// Still returns 200 even if PG is down
	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "disconnected", body["pg"])
}

func TestHealthHandler_Handle_InvalidNATSURL(t *testing.T) {
	pool := newTestPool(t)

	// Use a definitely-unreachable NATS address
	h := handlers.NewHealthHandler(pool, "nats://192.0.2.1:4222")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "connected", body["pg"])
	assert.Equal(t, "disconnected", body["nats"])
}

func TestHealthHandler_Handle_EmptyNATSURL(t *testing.T) {
	pool := newTestPool(t)

	h := handlers.NewHealthHandler(pool, "")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var body map[string]string
	err := json.NewDecoder(rec.Body).Decode(&body)
	require.NoError(t, err)

	assert.Equal(t, "disconnected", body["nats"])
}
