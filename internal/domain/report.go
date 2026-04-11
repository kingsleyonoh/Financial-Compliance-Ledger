package domain

import "time"

// Report type constants represent the kinds of compliance reports.
const (
	ReportTypeDailySummary      = "daily_summary"
	ReportTypeMonthlyAudit      = "monthly_audit"
	ReportTypeDiscrepancyDetail = "discrepancy_detail"
	ReportTypeCustom            = "custom"
)

// Report status constants represent the lifecycle of report generation.
const (
	ReportStatusPending    = "pending"
	ReportStatusGenerating = "generating"
	ReportStatusCompleted  = "completed"
	ReportStatusFailed     = "failed"
	ReportStatusCleaned    = "cleaned"
)

// validReportTypes is the set of all allowed report types.
var validReportTypes = map[string]bool{
	ReportTypeDailySummary:      true,
	ReportTypeMonthlyAudit:      true,
	ReportTypeDiscrepancyDetail: true,
	ReportTypeCustom:            true,
}

// validReportStatuses is the set of all allowed report statuses.
var validReportStatuses = map[string]bool{
	ReportStatusPending:    true,
	ReportStatusGenerating: true,
	ReportStatusCompleted:  true,
	ReportStatusFailed:     true,
	ReportStatusCleaned:    true,
}

// Report represents metadata for a generated compliance report.
// Actual PDF files are stored on disk at FilePath.
type Report struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenant_id"`
	ReportType    string                 `json:"report_type"`
	Title         string                 `json:"title"`
	Parameters    map[string]interface{} `json:"parameters"`
	Status        string                 `json:"status"`
	FilePath      *string                `json:"file_path,omitempty"`
	FileSizeBytes *int64                 `json:"file_size_bytes,omitempty"`
	GeneratedBy   *string                `json:"generated_by,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// ValidReportType returns true if the given string is a valid report type.
func ValidReportType(t string) bool {
	return validReportTypes[t]
}

// ValidReportStatus returns true if the given string is a valid report status.
func ValidReportStatus(s string) bool {
	return validReportStatuses[s]
}
