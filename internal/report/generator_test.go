package report

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

// ---------- ReportData aggregation tests ----------

func TestAggregateStats_BasicCounts(t *testing.T) {
	now := time.Now()
	resolved := now.Add(-1 * time.Hour)
	discs := []*domain.Discrepancy{
		{ID: "d1", Severity: domain.SeverityHigh, Status: domain.StatusOpen, CreatedAt: now},
		{ID: "d2", Severity: domain.SeverityLow, Status: domain.StatusResolved, CreatedAt: now.Add(-2 * time.Hour), ResolvedAt: &resolved},
		{ID: "d3", Severity: domain.SeverityHigh, Status: domain.StatusEscalated, CreatedAt: now},
	}
	events := []*domain.LedgerEvent{
		{ID: "e1", DiscrepancyID: "d1", EventType: domain.EventReceived},
		{ID: "e2", DiscrepancyID: "d2", EventType: domain.EventResolved},
	}

	data := aggregateReportData(discs, events, 10000)

	assert.Equal(t, 3, data.TotalDiscrepancies)
	assert.Equal(t, 3, data.ByStatus[domain.StatusOpen]+data.ByStatus[domain.StatusResolved]+data.ByStatus[domain.StatusEscalated])
	assert.Equal(t, 1, data.ByStatus[domain.StatusOpen])
	assert.Equal(t, 1, data.ByStatus[domain.StatusResolved])
	assert.Equal(t, 1, data.ByStatus[domain.StatusEscalated])
	assert.Equal(t, 2, data.BySeverity[domain.SeverityHigh])
	assert.Equal(t, 1, data.BySeverity[domain.SeverityLow])
	assert.Equal(t, 2, data.TotalEvents)
	assert.False(t, data.EventsTruncated)
}

func TestAggregateStats_EventTruncation(t *testing.T) {
	discs := []*domain.Discrepancy{
		{ID: "d1", Status: domain.StatusOpen, Severity: domain.SeverityLow},
	}

	// Create 15 events but max is 10
	events := make([]*domain.LedgerEvent, 15)
	for i := 0; i < 15; i++ {
		events[i] = &domain.LedgerEvent{
			ID:            uuid.New().String(),
			DiscrepancyID: "d1",
			EventType:     domain.EventReceived,
		}
	}

	data := aggregateReportData(discs, events, 10)

	assert.True(t, data.EventsTruncated)
	assert.Equal(t, 15, data.OriginalEventCount)
	assert.Equal(t, 10, len(data.Events))
}

func TestAggregateStats_NoDiscrepancies(t *testing.T) {
	data := aggregateReportData(nil, nil, 10000)

	assert.Equal(t, 0, data.TotalDiscrepancies)
	assert.Equal(t, 0, data.TotalEvents)
	assert.False(t, data.EventsTruncated)
	assert.NotNil(t, data.ByStatus)
	assert.NotNil(t, data.BySeverity)
}

func TestAggregateStats_MeanResolutionTime(t *testing.T) {
	now := time.Now()
	created1 := now.Add(-4 * time.Hour)
	resolved1 := now.Add(-2 * time.Hour) // 2 hours to resolve
	created2 := now.Add(-6 * time.Hour)
	resolved2 := now.Add(-2 * time.Hour) // 4 hours to resolve

	discs := []*domain.Discrepancy{
		{ID: "d1", Severity: domain.SeverityHigh, Status: domain.StatusResolved, CreatedAt: created1, ResolvedAt: &resolved1},
		{ID: "d2", Severity: domain.SeverityLow, Status: domain.StatusResolved, CreatedAt: created2, ResolvedAt: &resolved2},
		{ID: "d3", Severity: domain.SeverityMedium, Status: domain.StatusOpen, CreatedAt: now}, // unresolved — excluded
	}

	data := aggregateReportData(discs, nil, 10000)

	// Mean resolution = (2h + 4h) / 2 = 3h
	assert.InDelta(t, 3.0, data.MeanResolutionHours, 0.01)
}

func TestAggregateStats_ByEventType(t *testing.T) {
	events := []*domain.LedgerEvent{
		{ID: "e1", EventType: domain.EventReceived},
		{ID: "e2", EventType: domain.EventReceived},
		{ID: "e3", EventType: domain.EventEscalated},
		{ID: "e4", EventType: domain.EventResolved},
	}

	data := aggregateReportData(nil, events, 10000)

	assert.Equal(t, 2, data.ByEventType[domain.EventReceived])
	assert.Equal(t, 1, data.ByEventType[domain.EventEscalated])
	assert.Equal(t, 1, data.ByEventType[domain.EventResolved])
}

// ---------- Template rendering tests ----------

func TestRenderTemplate_DailySummary(t *testing.T) {
	data := &ReportData{
		TenantName:         "Acme Corp",
		ReportTitle:        "Daily Summary",
		DateFrom:           "2024-01-01",
		DateTo:             "2024-01-01",
		GeneratedAt:        time.Now().Format(time.RFC3339),
		TotalDiscrepancies: 5,
		ByStatus:           map[string]int{"open": 3, "resolved": 2},
		BySeverity:         map[string]int{"high": 2, "low": 3},
		Discrepancies:      []*domain.Discrepancy{},
		Events:             []*domain.LedgerEvent{},
		ByEventType:        map[string]int{},
		TotalEvents:        10,
	}

	var buf bytes.Buffer
	err := renderTemplate(&buf, domain.ReportTypeDailySummary, data)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Acme Corp")
	assert.Contains(t, html, "Daily Summary")
	assert.Contains(t, html, "2024-01-01")
	assert.Contains(t, html, "Financial Compliance Ledger")
}

func TestRenderTemplate_DailySummary_NoDiscrepancies(t *testing.T) {
	data := &ReportData{
		TenantName:         "Acme Corp",
		ReportTitle:        "Daily Summary",
		DateFrom:           "2024-01-01",
		DateTo:             "2024-01-01",
		GeneratedAt:        time.Now().Format(time.RFC3339),
		TotalDiscrepancies: 0,
		ByStatus:           map[string]int{},
		BySeverity:         map[string]int{},
		Discrepancies:      []*domain.Discrepancy{},
		Events:             []*domain.LedgerEvent{},
		ByEventType:        map[string]int{},
	}

	var buf bytes.Buffer
	err := renderTemplate(&buf, domain.ReportTypeDailySummary, data)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "No discrepancies found")
}

func TestRenderTemplate_MonthlyAudit(t *testing.T) {
	data := &ReportData{
		TenantName:          "Acme Corp",
		ReportTitle:         "Monthly Audit",
		DateFrom:            "2024-01-01",
		DateTo:              "2024-01-31",
		GeneratedAt:         time.Now().Format(time.RFC3339),
		TotalDiscrepancies:  10,
		ByStatus:            map[string]int{"open": 3, "resolved": 7},
		BySeverity:          map[string]int{"high": 5, "low": 5},
		MeanResolutionHours: 2.5,
		ByEventType:         map[string]int{"discrepancy.received": 10, "discrepancy.escalated": 3},
		Discrepancies:       []*domain.Discrepancy{},
		Events:              []*domain.LedgerEvent{},
		TotalEvents:         20,
	}

	var buf bytes.Buffer
	err := renderTemplate(&buf, domain.ReportTypeMonthlyAudit, data)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Monthly Audit")
	assert.Contains(t, html, "2.5")
	assert.Contains(t, html, "Financial Compliance Ledger")
}

func TestRenderTemplate_DiscrepancyDetail(t *testing.T) {
	now := time.Now()
	data := &ReportData{
		TenantName:         "Acme Corp",
		ReportTitle:        "Discrepancy Detail",
		DateFrom:           "2024-01-01",
		DateTo:             "2024-01-31",
		GeneratedAt:        now.Format(time.RFC3339),
		TotalDiscrepancies: 1,
		ByStatus:           map[string]int{"open": 1},
		BySeverity:         map[string]int{"high": 1},
		ByEventType:        map[string]int{},
		Discrepancies: []*domain.Discrepancy{
			{
				ID:        "disc-001",
				Severity:  domain.SeverityHigh,
				Status:    domain.StatusOpen,
				Title:     "Missing transaction",
				CreatedAt: now,
			},
		},
		Events: []*domain.LedgerEvent{
			{
				ID:        "evt-001",
				EventType: domain.EventReceived,
				Actor:     "system",
				CreatedAt: now,
			},
		},
		TotalEvents: 1,
	}

	var buf bytes.Buffer
	err := renderTemplate(&buf, domain.ReportTypeDiscrepancyDetail, data)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "Missing transaction")
	assert.Contains(t, html, "disc-001")
}

func TestRenderTemplate_EventTruncationNote(t *testing.T) {
	data := &ReportData{
		TenantName:         "Acme Corp",
		ReportTitle:        "Daily Summary",
		DateFrom:           "2024-01-01",
		DateTo:             "2024-01-01",
		GeneratedAt:        time.Now().Format(time.RFC3339),
		TotalDiscrepancies: 1,
		ByStatus:           map[string]int{"open": 1},
		BySeverity:         map[string]int{"high": 1},
		ByEventType:        map[string]int{},
		Discrepancies:      []*domain.Discrepancy{},
		Events:             []*domain.LedgerEvent{},
		TotalEvents:        100,
		EventsTruncated:    true,
		OriginalEventCount: 15000,
	}

	var buf bytes.Buffer
	err := renderTemplate(&buf, domain.ReportTypeDailySummary, data)
	require.NoError(t, err)

	html := buf.String()
	assert.Contains(t, html, "10,000")
	assert.Contains(t, html, "15,000")
}

func TestRenderTemplate_InvalidType_ReturnsError(t *testing.T) {
	data := &ReportData{}
	var buf bytes.Buffer
	err := renderTemplate(&buf, "nonexistent_type", data)
	assert.Error(t, err)
}

// ---------- File saving tests ----------

func TestSaveReportFile_CreatesDirectoryAndFile(t *testing.T) {
	tmpDir := t.TempDir()
	tenantID := uuid.New()
	reportID := uuid.New()

	content := []byte("<html><body>Test Report</body></html>")
	filePath, fileSize, err := saveReportFile(tmpDir, tenantID.String(), reportID.String(), content, ".html")
	require.NoError(t, err)

	assert.FileExists(t, filePath)
	assert.Equal(t, int64(len(content)), fileSize)

	// Verify path structure
	expectedDir := filepath.Join(tmpDir, tenantID.String())
	assert.DirExists(t, expectedDir)
	assert.Contains(t, filePath, reportID.String())
	assert.Contains(t, filePath, ".html")
}

// ---------- wkhtmltopdf availability test ----------

func TestCheckWkhtmltopdfAvailable(t *testing.T) {
	// This test just verifies the function runs without panicking.
	// It will return true or false depending on the environment.
	result := checkWkhtmltopdfAvailable()
	// On CI/dev machines wkhtmltopdf may not be installed
	t.Logf("wkhtmltopdf available: %v", result)
	assert.IsType(t, true, result)
}

// ---------- NewReportGenerator ----------

func TestNewReportGenerator(t *testing.T) {
	logger := zerolog.New(os.Stderr)
	gen := NewReportGenerator(nil, nil, nil, "data/reports", 10000, logger)

	assert.NotNil(t, gen)
	assert.Equal(t, "data/reports", gen.storagePath)
	assert.Equal(t, 10000, gen.maxEvents)
}
