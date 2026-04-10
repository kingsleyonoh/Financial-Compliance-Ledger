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
)

// RouterDeps holds all dependencies needed to construct the router.
// Fields are nil-safe: if Pool is nil, tenant middleware is skipped
// (useful for unit tests that don't need DB).
type RouterDeps struct {
	Pool   *pgxpool.Pool
	Logger zerolog.Logger
}

// NewRouter creates a configured Chi router with all route groups,
// middleware, and placeholder handlers.
func NewRouter(deps RouterDeps) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(mw.RequestLogger(deps.Logger))
	r.Use(chimw.Recoverer)

	// Public routes
	r.Get("/health", healthHandler)

	// Public tenant registration (no auth)
	r.Route("/api/tenants", func(r chi.Router) {
		r.Post("/register", placeholderHandler)
	})

	// Authenticated API routes
	r.Route("/api", func(r chi.Router) {
		// Apply tenant auth middleware only if pool is available
		if deps.Pool != nil {
			tm := mw.NewTenantMiddleware(deps.Pool)
			r.Use(tm.Handler)
		} else {
			// In tests without DB, reject with 401 for auth-required routes
			r.Use(requireAuthStub)
		}

		r.Route("/discrepancies", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Put("/{id}/status", placeholderHandler)
		})

		r.Route("/rules", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Put("/{id}", placeholderHandler)
			r.Delete("/{id}", placeholderHandler)
		})

		r.Route("/reports", func(r chi.Router) {
			r.Get("/", placeholderHandler)
			r.Post("/", placeholderHandler)
			r.Get("/{id}", placeholderHandler)
			r.Get("/{id}/download", placeholderHandler)
		})

		r.Route("/stats", func(r chi.Router) {
			r.Get("/", placeholderHandler)
		})
	})

	return r
}

// healthHandler returns a simple health check response.
func healthHandler(w http.ResponseWriter, r *http.Request) {
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
