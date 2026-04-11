package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/engine"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

func TestMetricsCollector_Snapshot(t *testing.T) {
	pool := newTestPool(t)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	tenantID := seedTenant(t, pool, "metrics-snap")
	t.Cleanup(func() { cleanupTenantData(t, pool, tenantID) })

	ctx := context.Background()
	ds := store.NewDiscrepancyStore(pool)

	// Seed some discrepancies with varying status and severity
	for _, status := range []string{domain.StatusOpen, domain.StatusOpen, domain.StatusResolved} {
		disc := &domain.Discrepancy{
			ExternalID:      "ext-metrics-" + status + "-" + time.Now().Format("150405.000000000"),
			SourceSystem:    "test",
			DiscrepancyType: domain.TypeMismatch,
			Severity:        domain.SeverityHigh,
			Status:          status,
			Title:           "Metrics test " + status,
			Currency:        "USD",
			Metadata:        map[string]interface{}{},
		}
		_, err := ds.Create(ctx, tenantID, disc)
		if err != nil {
			t.Fatalf("failed to seed discrepancy: %v", err)
		}
	}

	collector := engine.NewMetricsCollector(pool, logger)

	// Snapshot should succeed without error
	err := collector.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
}

func TestMetricsCollector_Start_StopsOnCancel(t *testing.T) {
	pool := newTestPool(t)
	logger := zerolog.New(zerolog.NewTestWriter(t))

	collector := engine.NewMetricsCollector(pool, logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		collector.Start(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not stop after context cancellation")
	}
}
