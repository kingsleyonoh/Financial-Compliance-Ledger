// Package middleware provides HTTP middleware and context helpers
// for the Financial Compliance Ledger API.
package middleware

import (
	"context"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/ctxutil"
)

// SetTenantID stores the tenant ID in the context.
// Delegates to ctxutil to avoid import cycles.
func SetTenantID(ctx context.Context, tenantID string) context.Context {
	return ctxutil.SetTenantID(ctx, tenantID)
}

// GetTenantID retrieves the tenant ID from the context.
// Returns an empty string if not set.
func GetTenantID(ctx context.Context) string {
	return ctxutil.GetTenantID(ctx)
}

// SetRequestID stores the request ID in the context.
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return ctxutil.SetRequestID(ctx, requestID)
}

// GetRequestID retrieves the request ID from the context.
// Returns an empty string if not set.
func GetRequestID(ctx context.Context) string {
	return ctxutil.GetRequestID(ctx)
}
