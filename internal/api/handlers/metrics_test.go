package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
)

func TestMetricsHandler_ReturnsPrometheusFormat(t *testing.T) {
	h := handlers.NewMetricsHandler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// Prometheus metrics endpoint should contain at least process stats
	assert.Contains(t, body, "# HELP", "response should contain Prometheus help text")
	assert.Contains(t, body, "# TYPE", "response should contain Prometheus type info")
}

func TestMetricsHandler_CustomMetricsRegistered(t *testing.T) {
	h := handlers.NewMetricsHandler()

	// Increment a custom metric to verify it appears
	handlers.LedgerEventsTotal.WithLabelValues("discrepancy.received").Inc()
	handlers.EscalationActionsTotal.WithLabelValues("notify").Inc()
	handlers.ReportsGeneratedTotal.WithLabelValues("daily_summary").Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	h.Handle(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "ledger_events_total",
		"should include ledger_events_total metric")
	assert.Contains(t, body, "escalation_actions_total",
		"should include escalation_actions_total metric")
	assert.Contains(t, body, "reports_generated_total",
		"should include reports_generated_total metric")
}
