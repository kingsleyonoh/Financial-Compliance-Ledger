// Package api provides the HTTP router and route registration for the
// Financial Compliance Ledger API.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
	mw "github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/middleware"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
)

// RouterDeps holds all dependencies needed to construct the router.
// Fields are nil-safe: if Pool is nil, tenant middleware is skipped
// (useful for unit tests that don't need DB).
type RouterDeps struct {
	Pool          *pgxpool.Pool
	Logger        zerolog.Logger
	Config        *config.Config
	HealthHandler *handlers.HealthHandler
	TenantHandler *handlers.TenantHandler
}

// NewRouter creates a configured Chi router with all route groups,
// middleware, and handlers.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(mw.RequestLogger(deps.Logger))
	r.Use(chimw.Recoverer)

	// Public routes (no auth required)
	r.Get("/health", healthRoute(deps))
	r.Post("/api/tenants/register", registerRoute(deps))

	// Authenticated API routes
	r.Group(func(r chi.Router) {
		if deps.Pool != nil {
			tm := mw.NewTenantMiddleware(deps.Pool)
			r.Use(tm.Handler)
		} else {
			r.Use(requireAuthStub)
		}

		r.Get("/api/tenants/me", tenantMeRoute(deps))

		r.Route("/api/discrepancies", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Put("/{id}/status", placeholderHandler)
		})

		r.Route("/api/rules", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Put("/{id}", placeholderHandler)
			r.Delete("/{id}", placeholderHandler)
		})

		r.Route("/api/reports", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Get("/{id}/download", placeholderHandler)
		})

		r.Get("/api/stats", placeholderHandler)
	})

	return r
}

// healthRoute returns the health handler or a default.
func healthRoute(deps RouterDeps) http.HandlerFunc {
	if deps.HealthHandler != nil {
		return deps.HealthHandler.Handle
	}
	return defaultHealthHandler
}

// registerRoute returns the register handler or a placeholder.
func registerRoute(deps RouterDeps) http.HandlerFunc {
	if deps.TenantHandler != nil {
		return deps.TenantHandler.Register
	}
	return placeholderHandler
}

// tenantMeRoute returns the GetMe handler or a placeholder.
func tenantMeRoute(deps RouterDeps) http.HandlerFunc {
	if deps.TenantHandler != nil {
		return deps.TenantHandler.GetMe
	}
	return placeholderHandler
}

// defaultHealthHandler returns a simple health check when no
// HealthHandler is configured (backward compatibility).
func defaultHealthHandler(w http.ResponseWriter, r *http.Request) {
	handlers.RespondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// placeholderHandler returns 501 Not Implemented for routes that will be
// implemented in future batches.
func placeholderHandler(w http.ResponseWriter, r *http.Request) {
	handlers.RespondError(w, http.StatusNotImplemented,
		"NOT_IMPLEMENTED", "This endpoint is not yet implemented")
}

// requireAuthStub is a middleware that rejects all requests with 401
// when no DB pool is available (for router tests without a database).
func requireAuthStub(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.RespondError(w, http.StatusUnauthorized,
			"MISSING_API_KEY", "X-API-Key header is required")
	})
}
