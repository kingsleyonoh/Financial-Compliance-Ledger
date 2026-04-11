// Package main generates and publishes realistic test discrepancy events
// to NATS JetStream for local development and testing.
//
// Usage:
//
//	go run scripts/generate_test_events.go [-nats nats://localhost:4222] [-tenant TENANT_UUID] [-count 10]
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const subject = "recon.discrepancy.detected"

// testEvent is the envelope format matching ingestion_handler.go.
type testEvent struct {
	EventType string      `json:"event_type"`
	Source    string      `json:"source"`
	TenantID string      `json:"tenant_id"`
	Timestamp string      `json:"timestamp"`
	Payload  testPayload `json:"payload"`
}

// testPayload holds the discrepancy details.
type testPayload struct {
	ExternalID      string                 `json:"external_id"`
	SourceSystem    string                 `json:"source_system"`
	DiscrepancyType string                 `json:"discrepancy_type"`
	Severity        string                 `json:"severity"`
	Title           string                 `json:"title"`
	Description     string                 `json:"description"`
	AmountExpected  float64                `json:"amount_expected"`
	AmountActual    float64                `json:"amount_actual"`
	Currency        string                 `json:"currency"`
	Metadata        map[string]interface{} `json:"metadata"`
}

var discrepancyTypes = []string{
	"unmatched_gateway",
	"unmatched_ledger",
	"amount_mismatch",
	"date_mismatch",
}

var sourceSystems = []string{
	"stripe-reconciler",
	"paypal-reconciler",
	"bank-statement-parser",
	"internal-ledger",
}

var currencies = []string{"USD", "EUR", "GBP"}

func main() {
	natsURL := flag.String("nats", "nats://localhost:4222", "NATS server URL")
	tenantID := flag.String("tenant", "", "Tenant UUID (required)")
	count := flag.Int("count", 10, "Number of events to generate")
	flag.Parse()

	if *tenantID == "" {
		fmt.Fprintln(os.Stderr, "Error: -tenant flag is required")
		fmt.Fprintln(os.Stderr, "Usage: go run scripts/generate_test_events.go -tenant <UUID>")
		os.Exit(1)
	}

	if _, err := uuid.Parse(*tenantID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid tenant UUID: %v\n", err)
		os.Exit(1)
	}

	nc, err := nats.Connect(*natsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to NATS at %s: %v\n", *natsURL, err)
		os.Exit(1)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating JetStream context: %v\n", err)
		os.Exit(1)
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < *count; i++ {
		event := generateEvent(rng, *tenantID, i)
		data, err := json.Marshal(event)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling event %d: %v\n", i, err)
			continue
		}

		_, err = js.Publish(subject, data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing event %d: %v\n", i, err)
			continue
		}

		fmt.Printf("[%d/%d] Published %s | severity=%s | amount=%.2f→%.2f\n",
			i+1, *count, event.Payload.DiscrepancyType,
			event.Payload.Severity,
			event.Payload.AmountExpected, event.Payload.AmountActual)
	}

	fmt.Printf("\nDone: published %d events to %s\n", *count, subject)
}

func generateEvent(rng *rand.Rand, tenantID string, index int) testEvent {
	discType := discrepancyTypes[rng.Intn(len(discrepancyTypes))]
	source := sourceSystems[rng.Intn(len(sourceSystems))]
	currency := currencies[rng.Intn(len(currencies))]

	expected := roundTo2(rng.Float64() * 50000)
	diff := roundTo2(rng.Float64() * expected * 0.3)
	actual := roundTo2(expected - diff)
	if rng.Intn(2) == 0 {
		actual = roundTo2(expected + diff)
	}

	severity := deriveSeverity(absDiff(expected, actual))

	return testEvent{
		EventType: "discrepancy.detected",
		Source:    "test-event-generator",
		TenantID: tenantID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Payload: testPayload{
			ExternalID:      fmt.Sprintf("test-%s-%d", uuid.New().String()[:8], index),
			SourceSystem:    source,
			DiscrepancyType: discType,
			Severity:        severity,
			Title:           fmt.Sprintf("Test %s from %s", discType, source),
			Description:     fmt.Sprintf("Auto-generated test event #%d", index),
			AmountExpected:  expected,
			AmountActual:    actual,
			Currency:        currency,
			Metadata: map[string]interface{}{
				"generated_by": "test-event-generator",
				"index":        index,
				"timestamp":    time.Now().Unix(),
			},
		},
	}
}

// deriveSeverity determines severity based on amount difference thresholds.
// <$100=low, <$1000=medium, <$10000=high, >=$10000=critical
func deriveSeverity(amount float64) string {
	switch {
	case amount < 100:
		return "low"
	case amount < 1000:
		return "medium"
	case amount < 10000:
		return "high"
	default:
		return "critical"
	}
}

func absDiff(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

func roundTo2(f float64) float64 {
	return float64(int(f*100)) / 100
}
