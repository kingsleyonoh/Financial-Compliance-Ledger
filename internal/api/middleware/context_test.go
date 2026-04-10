package middleware_test

import (
	"context"
	"testing"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/middleware"
	"github.com/stretchr/testify/assert"
)

func TestSetAndGetTenantID(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-abc-123"

	ctx = middleware.SetTenantID(ctx, tenantID)
	got := middleware.GetTenantID(ctx)

	assert.Equal(t, tenantID, got)
}

func TestGetTenantID_ReturnsEmptyWhenNotSet(t *testing.T) {
	ctx := context.Background()

	got := middleware.GetTenantID(ctx)

	assert.Equal(t, "", got)
}

func TestSetAndGetRequestID(t *testing.T) {
	ctx := context.Background()
	requestID := "req-xyz-789"

	ctx = middleware.SetRequestID(ctx, requestID)
	got := middleware.GetRequestID(ctx)

	assert.Equal(t, requestID, got)
}

func TestGetRequestID_ReturnsEmptyWhenNotSet(t *testing.T) {
	ctx := context.Background()

	got := middleware.GetRequestID(ctx)

	assert.Equal(t, "", got)
}

func TestMultipleContextValues_DoNotInterfere(t *testing.T) {
	ctx := context.Background()
	tenantID := "tenant-001"
	requestID := "req-002"

	ctx = middleware.SetTenantID(ctx, tenantID)
	ctx = middleware.SetRequestID(ctx, requestID)

	assert.Equal(t, tenantID, middleware.GetTenantID(ctx))
	assert.Equal(t, requestID, middleware.GetRequestID(ctx))
}

func TestSetTenantID_OverwritesPreviousValue(t *testing.T) {
	ctx := context.Background()

	ctx = middleware.SetTenantID(ctx, "tenant-old")
	ctx = middleware.SetTenantID(ctx, "tenant-new")

	assert.Equal(t, "tenant-new", middleware.GetTenantID(ctx))
}
