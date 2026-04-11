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
	Pool                *pgxpool.Pool
	Logger              zerolog.Logger
	Config              *config.Config
	HealthHandler       *handlers.HealthHandler
	TenantHandler       *handlers.TenantHandler
	DiscrepancyHandler  *handlers.DiscrepancyHandler
	RulesHandler        *handlers.RulesHandler
	StatsHandler        *handlers.StatsHandler
	ReportsHandler      *handlers.ReportsHandler
	MetricsHandler      *handlers.MetricsHandler
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
	r.Get("/metrics", metricsRoute(deps))
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
			r.Get("/", discrepancyListRoute(deps))
			r.Post("/", placeholderHandler)
			r.Get("/{id}", discrepancyGetRoute(deps))
			r.Put("/{id}/status", placeholderHandler)
			r.Post("/{id}/acknowledge", discrepancyAcknowledgeRoute(deps))
			r.Post("/{id}/investigate", discrepancyInvestigateRoute(deps))
			r.Post("/{id}/resolve", discrepancyResolveRoute(deps))
			r.Post("/{id}/notes", discrepancyAddNoteRoute(deps))
		})

		r.Route("/api/rules", func(r chi.Router) {
			r.Get("/", rulesListRoute(deps))
			r.Post("/", rulesCreateRoute(deps))
			r.Get("/{id}", placeholderHandler)
			r.Put("/{id}", rulesUpdateRoute(deps))
			r.Delete("/{id}", rulesDeleteRoute(deps))
		})

		r.Route("/api/reports", func(r chi.Router) {
			r.Get("/", reportsListRoute(deps))
			r.Post("/", reportsCreateRoute(deps))
			r.Get("/{id}", placeholderHandler)
			r.Get("/{id}/download", reportsDownloadRoute(deps))
		})

		r.Get("/api/stats", statsRoute(deps))
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

// discrepancyListRoute returns the list handler or a placeholder.
func discrepancyListRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.List
	}
	return placeholderHandler
}

// discrepancyGetRoute returns the get-by-ID handler or a placeholder.
func discrepancyGetRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.GetByID
	}
	return placeholderHandler
}

// discrepancyAcknowledgeRoute returns the acknowledge handler or a placeholder.
func discrepancyAcknowledgeRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.Acknowledge
	}
	return placeholderHandler
}

// discrepancyInvestigateRoute returns the investigate handler or a placeholder.
func discrepancyInvestigateRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.Investigate
	}
	return placeholderHandler
}

// discrepancyResolveRoute returns the resolve handler or a placeholder.
func discrepancyResolveRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.Resolve
	}
	return placeholderHandler
}

// discrepancyAddNoteRoute returns the add-note handler or a placeholder.
func discrepancyAddNoteRoute(deps RouterDeps) http.HandlerFunc {
	if deps.DiscrepancyHandler != nil {
		return deps.DiscrepancyHandler.AddNote
	}
	return placeholderHandler
}

// statsRoute returns the stats handler or a placeholder.
func statsRoute(deps RouterDeps) http.HandlerFunc {
	if deps.StatsHandler != nil {
		return deps.StatsHandler.GetStats
	}
	return placeholderHandler
}

// rulesListRoute returns the rules list handler or a placeholder.
func rulesListRoute(deps RouterDeps) http.HandlerFunc {
	if deps.RulesHandler != nil {
		return deps.RulesHandler.List
	}
	return placeholderHandler
}

// rulesCreateRoute returns the rules create handler or a placeholder.
func rulesCreateRoute(deps RouterDeps) http.HandlerFunc {
	if deps.RulesHandler != nil {
		return deps.RulesHandler.Create
	}
	return placeholderHandler
}

// rulesUpdateRoute returns the rules update handler or a placeholder.
func rulesUpdateRoute(deps RouterDeps) http.HandlerFunc {
	if deps.RulesHandler != nil {
		return deps.RulesHandler.Update
	}
	return placeholderHandler
}

// rulesDeleteRoute returns the rules delete handler or a placeholder.
func rulesDeleteRoute(deps RouterDeps) http.HandlerFunc {
	if deps.RulesHandler != nil {
		return deps.RulesHandler.Delete
	}
	return placeholderHandler
}

// reportsListRoute returns the reports list handler or a placeholder.
func reportsListRoute(deps RouterDeps) http.HandlerFunc {
	if deps.ReportsHandler != nil {
		return deps.ReportsHandler.List
	}
	return placeholderHandler
}

// reportsCreateRoute returns the reports create handler or a placeholder.
func reportsCreateRoute(deps RouterDeps) http.HandlerFunc {
	if deps.ReportsHandler != nil {
		return deps.ReportsHandler.Create
	}
	return placeholderHandler
}

// reportsDownloadRoute returns the report download handler or a placeholder.
func reportsDownloadRoute(deps RouterDeps) http.HandlerFunc {
	if deps.ReportsHandler != nil {
		return deps.ReportsHandler.Download
	}
	return placeholderHandler
}

// metricsRoute returns the Prometheus metrics handler or a placeholder.
func metricsRoute(deps RouterDeps) http.HandlerFunc {
	if deps.MetricsHandler != nil {
		return deps.MetricsHandler.Handle
	}
	return placeholderHandler
}

// requireAuthStub is a middleware that rejects all requests with 401
// when no DB pool is available (for router tests without a database).
func requireAuthStub(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.RespondError(w, http.StatusUnauthorized,
			"MISSING_API_KEY", "X-API-Key header is required")
	})
}
