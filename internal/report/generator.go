package report

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

// ReportGenerator orchestrates report data aggregation, template
// rendering, and file output (HTML or PDF).
type ReportGenerator struct {
	discrepancyStore *store.DiscrepancyStore
	eventStore       *store.EventStore
	reportStore      *store.ReportStore
	storagePath      string
	maxEvents        int
	logger           zerolog.Logger
}

// NewReportGenerator creates a new ReportGenerator with the given
// dependencies.
func NewReportGenerator(
	ds *store.DiscrepancyStore,
	es *store.EventStore,
	rs *store.ReportStore,
	storagePath string,
	maxEvents int,
	logger zerolog.Logger,
) *ReportGenerator {
	return &ReportGenerator{
		discrepancyStore: ds,
		eventStore:       es,
		reportStore:      rs,
		storagePath:      storagePath,
		maxEvents:        maxEvents,
		logger:           logger,
	}
}

// Generate performs the full report generation lifecycle:
// 1. Mark report as "generating"
// 2. Query discrepancies and events for the date range
// 3. Aggregate statistics
// 4. Render HTML template
// 5. Convert to PDF (or fall back to HTML)
// 6. Save file and update report status
func (g *ReportGenerator) Generate(
	ctx context.Context, tenantID uuid.UUID, report *domain.Report,
) error {
	reportID, err := uuid.Parse(report.ID)
	if err != nil {
		return fmt.Errorf("invalid report ID: %w", err)
	}

	// Step 1: Mark as generating
	if err := g.reportStore.UpdateStatus(
		ctx, tenantID, reportID, domain.ReportStatusGenerating, nil, nil,
	); err != nil {
		return g.failReport(ctx, tenantID, reportID, "update status to generating", err)
	}

	// Step 2: Parse date range from parameters
	dateFrom, dateTo, err := parseDateRange(report.Parameters)
	if err != nil {
		return g.failReport(ctx, tenantID, reportID, "parse date range", err)
	}

	// Step 3: Query discrepancies
	filters := store.ListFilters{
		DateFrom: &dateFrom,
		DateTo:   &dateTo,
		Limit:    100,
	}
	discs, _, err := g.discrepancyStore.List(ctx, tenantID, filters)
	if err != nil {
		return g.failReport(ctx, tenantID, reportID, "query discrepancies", err)
	}

	// Step 4: Query events for those discrepancies
	var allEvents []*domain.LedgerEvent
	for _, d := range discs {
		discID, parseErr := uuid.Parse(d.ID)
		if parseErr != nil {
			continue
		}
		events, queryErr := g.eventStore.ListByDiscrepancy(ctx, tenantID, discID)
		if queryErr != nil {
			g.logger.Warn().Err(queryErr).Str("discrepancy_id", d.ID).
				Msg("failed to query events for discrepancy")
			continue
		}
		allEvents = append(allEvents, events...)
	}

	// Step 5: Aggregate statistics
	data := aggregateReportData(discs, allEvents, g.maxEvents)
	data.TenantName = tenantID.String()
	data.ReportTitle = report.Title
	data.DateFrom = dateFrom.Format("2006-01-02")
	data.DateTo = dateTo.Format("2006-01-02")
	data.GeneratedAt = time.Now().Format(time.RFC3339)

	// Step 6: Render HTML template
	var htmlBuf bytes.Buffer
	reportType := report.ReportType
	if reportType == "" {
		reportType = domain.ReportTypeDailySummary
	}
	if err := renderTemplate(&htmlBuf, reportType, data); err != nil {
		return g.failReport(ctx, tenantID, reportID, "render template", err)
	}

	// Step 7: Convert to PDF or save as HTML
	filePath, fileSize, err := g.saveOutput(
		tenantID.String(), reportID.String(), htmlBuf.Bytes(),
	)
	if err != nil {
		return g.failReport(ctx, tenantID, reportID, "save output", err)
	}

	// Step 8: Update report status to completed
	if err := g.reportStore.UpdateStatus(
		ctx, tenantID, reportID, domain.ReportStatusCompleted, &filePath, &fileSize,
	); err != nil {
		return fmt.Errorf("update status to completed: %w", err)
	}

	g.logger.Info().
		Str("report_id", reportID.String()).
		Str("file_path", filePath).
		Int64("file_size", fileSize).
		Msg("report generated successfully")

	return nil
}

// saveOutput tries wkhtmltopdf first, falls back to HTML.
func (g *ReportGenerator) saveOutput(
	tenantID, reportID string, htmlContent []byte,
) (string, int64, error) {
	if checkWkhtmltopdfAvailable() {
		return g.saveAsPDF(tenantID, reportID, htmlContent)
	}
	g.logger.Warn().Msg("wkhtmltopdf not available, saving as HTML")
	return saveReportFile(g.storagePath, tenantID, reportID, htmlContent, ".html")
}

// saveAsPDF writes HTML to a temp file, converts to PDF, cleans up.
func (g *ReportGenerator) saveAsPDF(
	tenantID, reportID string, htmlContent []byte,
) (string, int64, error) {
	tmpFile, err := os.CreateTemp("", "report-*.html")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(htmlContent); err != nil {
		tmpFile.Close()
		return "", 0, fmt.Errorf("write temp file: %w", err)
	}
	tmpFile.Close()

	// Determine PDF output path
	pdfPath, _, err := saveReportFile(
		g.storagePath, tenantID, reportID, nil, ".pdf",
	)
	if err != nil {
		return "", 0, err
	}

	if err := convertHTMLToPDF(tmpFile.Name(), pdfPath); err != nil {
		os.Remove(pdfPath)
		// Fall back to HTML on conversion failure
		g.logger.Warn().Err(err).Msg("PDF conversion failed, falling back to HTML")
		return saveReportFile(g.storagePath, tenantID, reportID, htmlContent, ".html")
	}

	info, err := os.Stat(pdfPath)
	if err != nil {
		return "", 0, fmt.Errorf("stat PDF: %w", err)
	}

	return pdfPath, info.Size(), nil
}

// failReport marks a report as failed and logs the error.
func (g *ReportGenerator) failReport(
	ctx context.Context, tenantID, reportID uuid.UUID,
	step string, original error,
) error {
	g.logger.Error().Err(original).
		Str("report_id", reportID.String()).
		Str("step", step).
		Msg("report generation failed")

	_ = g.reportStore.UpdateStatus(
		ctx, tenantID, reportID, domain.ReportStatusFailed, nil, nil,
	)
	return fmt.Errorf("report generation failed at %s: %w", step, original)
}

// parseDateRange extracts date_from and date_to from report parameters.
func parseDateRange(
	params map[string]interface{},
) (time.Time, time.Time, error) {
	fromStr, ok := params["date_from"].(string)
	if !ok || fromStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("missing date_from parameter")
	}
	toStr, ok := params["date_to"].(string)
	if !ok || toStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("missing date_to parameter")
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date_from: %w", err)
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid date_to: %w", err)
	}

	// Set to end of day for date_to
	to = to.Add(23*time.Hour + 59*time.Minute + 59*time.Second)

	return from, to, nil
}
