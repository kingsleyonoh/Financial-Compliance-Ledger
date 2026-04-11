// Package report provides compliance report generation including data
// aggregation, HTML template rendering, and optional PDF conversion.
package report

import (
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ReportData holds all aggregated data needed to render a report template.
type ReportData struct {
	// Header fields
	TenantName  string `json:"tenant_name"`
	ReportTitle string `json:"report_title"`
	DateFrom    string `json:"date_from"`
	DateTo      string `json:"date_to"`
	GeneratedAt string `json:"generated_at"`

	// Summary statistics
	TotalDiscrepancies int            `json:"total_discrepancies"`
	ByStatus           map[string]int `json:"by_status"`
	BySeverity         map[string]int `json:"by_severity"`
	TotalEvents        int            `json:"total_events"`
	ByEventType        map[string]int `json:"by_event_type"`

	// Resolution statistics
	MeanResolutionHours float64 `json:"mean_resolution_hours"`

	// Discrepancy and event lists
	Discrepancies []*domain.Discrepancy `json:"discrepancies"`
	Events        []*domain.LedgerEvent `json:"events"`

	// Truncation info
	EventsTruncated    bool `json:"events_truncated"`
	OriginalEventCount int  `json:"original_event_count"`
}

// aggregateReportData computes summary statistics from discrepancies and
// events. If the event count exceeds maxEvents, the events slice is
// truncated and the truncation fields are set.
func aggregateReportData(
	discs []*domain.Discrepancy,
	events []*domain.LedgerEvent,
	maxEvents int,
) *ReportData {
	data := &ReportData{
		ByStatus:    make(map[string]int),
		BySeverity:  make(map[string]int),
		ByEventType: make(map[string]int),
	}

	// Aggregate discrepancy stats
	var totalResolutionHours float64
	var resolvedCount int

	for _, d := range discs {
		data.TotalDiscrepancies++
		data.ByStatus[d.Status]++
		data.BySeverity[d.Severity]++

		if d.ResolvedAt != nil && !d.ResolvedAt.IsZero() {
			hours := d.ResolvedAt.Sub(d.CreatedAt).Hours()
			totalResolutionHours += hours
			resolvedCount++
		}
	}

	if resolvedCount > 0 {
		data.MeanResolutionHours = totalResolutionHours / float64(resolvedCount)
	}

	// Aggregate event stats
	data.TotalEvents = len(events)
	data.OriginalEventCount = len(events)

	for _, e := range events {
		data.ByEventType[e.EventType]++
	}

	// Truncate events if needed
	if len(events) > maxEvents {
		data.EventsTruncated = true
		data.Events = events[:maxEvents]
	} else {
		data.Events = events
	}

	data.Discrepancies = discs
	if data.Discrepancies == nil {
		data.Discrepancies = make([]*domain.Discrepancy, 0)
	}
	if data.Events == nil {
		data.Events = make([]*domain.LedgerEvent, 0)
	}

	return data
}
