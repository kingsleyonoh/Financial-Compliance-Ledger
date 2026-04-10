// Package middleware provides HTTP middleware and context helpers
// for the Financial Compliance Ledger API.
package middleware

import "context"

// contextKey is an unexported type used for context value keys to prevent
// collisions with keys from other packages.
type contextKey string

const (
	tenantIDKey  contextKey = "tenant_id"
	requestIDKey contextKey = "request_id"
)

// SetTenantID stores the tenant ID in the context.
func SetTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantIDKey, tenantID)
}

// GetTenantID retrieves the tenant ID from the context.
// Returns an empty string if not set.
func GetTenantID(ctx context.Context) string {
	v, ok := ctx.Value(tenantIDKey).(string)
	if !ok {
		return ""
	}
	return v
}

// SetRequestID stores the request ID in the context.
func SetRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context.
// Returns an empty string if not set.
func GetRequestID(ctx context.Context) string {
	v, ok := ctx.Value(requestIDKey).(string)
	if !ok {
		return ""
	}
	return v
}
