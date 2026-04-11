package report

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
)

//go:embed templates/*.html
var templateFS embed.FS

// templateMap maps report types to template file names.
var templateMap = map[string]string{
	domain.ReportTypeDailySummary:      "templates/daily_summary.html",
	domain.ReportTypeMonthlyAudit:      "templates/monthly_audit.html",
	domain.ReportTypeDiscrepancyDetail: "templates/discrepancy_detail.html",
	domain.ReportTypeCustom:            "templates/daily_summary.html",
}

// templateFuncs provides helper functions for templates.
var templateFuncs = template.FuncMap{
	"formatNumber": formatNumber,
	"upper":        strings.ToUpper,
}

// renderTemplate renders the appropriate HTML template for the given report
// type into the provided writer. Returns an error if the template is not
// found or fails to execute.
func renderTemplate(w io.Writer, reportType string, data *ReportData) error {
	tmplFile, ok := templateMap[reportType]
	if !ok {
		return fmt.Errorf("unknown report type: %s", reportType)
	}

	tmpl, err := template.New(filepath.Base(tmplFile)).
		Funcs(templateFuncs).
		ParseFS(templateFS, tmplFile)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", tmplFile, err)
	}

	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("execute template %s: %w", tmplFile, err)
	}

	return nil
}

// saveReportFile writes content to disk at {storagePath}/{tenantID}/{reportID}{ext}.
// Creates the tenant directory if it does not exist. Returns the file path
// and file size.
func saveReportFile(
	storagePath, tenantID, reportID string,
	content []byte, ext string,
) (string, int64, error) {
	dir := filepath.Join(storagePath, tenantID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create report directory: %w", err)
	}

	fileName := reportID + ext
	filePath := filepath.Join(dir, fileName)

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return "", 0, fmt.Errorf("write report file: %w", err)
	}

	return filePath, int64(len(content)), nil
}

// checkWkhtmltopdfAvailable returns true if wkhtmltopdf is installed and
// accessible on the system PATH.
func checkWkhtmltopdfAvailable() bool {
	_, err := exec.LookPath("wkhtmltopdf")
	return err == nil
}

// convertHTMLToPDF converts an HTML file to PDF using wkhtmltopdf.
// Returns the PDF file path. The caller must ensure wkhtmltopdf is available.
func convertHTMLToPDF(htmlPath, pdfPath string) error {
	cmd := exec.Command("wkhtmltopdf", "--quiet", htmlPath, pdfPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wkhtmltopdf: %w: %s", err, string(output))
	}
	return nil
}

// formatNumber formats an integer with comma separators (e.g., 10000 -> "10,000").
func formatNumber(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
