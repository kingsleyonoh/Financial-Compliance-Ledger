package engine

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/domain"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/store"
)

const maxTimelineEvents = 50

// RAGFeeder pushes resolved discrepancies to the Multi-Agent RAG Platform
// for queryable compliance history. Feature-flagged via RAGFeedEnabled.
type RAGFeeder struct {
	baseURL          string
	apiKey           string
	enabled          bool
	discrepancyStore *store.DiscrepancyStore
	eventStore       *store.EventStore
	httpClient       *http.Client
	logger           zerolog.Logger
}

// NewRAGFeeder creates a new RAGFeeder from config.
func NewRAGFeeder(
	cfg *config.Config,
	ds *store.DiscrepancyStore,
	es *store.EventStore,
	logger zerolog.Logger,
) *RAGFeeder {
	return &RAGFeeder{
		baseURL:          cfg.RAGPlatformURL,
		apiKey:           cfg.RAGPlatformAPIKey,
		enabled:          cfg.RAGFeedEnabled,
		discrepancyStore: ds,
		eventStore:       es,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger.With().
			Str("component", "rag-feeder").Logger(),
	}
}

// FeedResolved formats a resolved discrepancy as markdown with YAML
// frontmatter and uploads it to the RAG Platform as a multipart file.
// Returns nil if disabled or if the RAG Platform is unreachable.
func (f *RAGFeeder) FeedResolved(
	ctx context.Context, tenantID uuid.UUID, disc *domain.Discrepancy,
) error {
	if !f.enabled {
		return nil
	}

	discID, err := uuid.Parse(disc.ID)
	if err != nil {
		return fmt.Errorf("rag_feeder.FeedResolved: invalid discrepancy ID: %w", err)
	}

	// Fetch event timeline
	events, err := f.eventStore.ListByDiscrepancy(ctx, tenantID, discID)
	if err != nil {
		f.logger.Warn().Err(err).
			Str("discrepancy_id", disc.ID).
			Msg("failed to fetch events for RAG feed")
		events = nil // proceed without timeline
	}

	// Truncate to last N events if too large
	if len(events) > maxTimelineEvents {
		events = events[len(events)-maxTimelineEvents:]
	}

	markdown := formatDiscrepancyMarkdown(disc, events)

	if err := f.uploadDocument(ctx, disc.ID, markdown); err != nil {
		f.logger.Warn().Err(err).
			Str("discrepancy_id", disc.ID).
			Msg("RAG Platform unreachable, skipping feed")
		return nil // non-critical — don't fail the caller
	}

	f.logger.Info().
		Str("discrepancy_id", disc.ID).
		Msg("fed resolved discrepancy to RAG Platform")
	return nil
}

// formatDiscrepancyMarkdown builds a markdown document with YAML
// frontmatter for the RAG Platform.
func formatDiscrepancyMarkdown(
	disc *domain.Discrepancy, events []*domain.LedgerEvent,
) []byte {
	var buf bytes.Buffer

	// Find resolution type from the last resolved/auto_closed event
	resolutionType := "unknown"
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].EventType == domain.EventResolved ||
			events[i].EventType == domain.EventAutoClosed {
			if rt, ok := events[i].Payload["resolution_type"].(string); ok {
				resolutionType = rt
			}
			break
		}
	}

	resolvedAt := "unknown"
	if disc.ResolvedAt != nil {
		resolvedAt = disc.ResolvedAt.Format(time.RFC3339)
	}

	// YAML frontmatter
	fmt.Fprintln(&buf, "---")
	fmt.Fprintf(&buf, "id: %s\n", disc.ID)
	fmt.Fprintf(&buf, "type: %s\n", disc.DiscrepancyType)
	fmt.Fprintf(&buf, "severity: %s\n", disc.Severity)
	fmt.Fprintf(&buf, "resolution: %s\n", resolutionType)
	fmt.Fprintf(&buf, "tenant_id: %s\n", disc.TenantID)
	fmt.Fprintf(&buf, "resolved_at: %s\n", resolvedAt)
	fmt.Fprintln(&buf, "---")

	// Body
	fmt.Fprintf(&buf, "# Discrepancy: %s\n", disc.Title)
	if disc.Description != nil {
		fmt.Fprintf(&buf, "%s\n", *disc.Description)
	}

	// Timeline
	fmt.Fprintln(&buf, "## Timeline")
	for _, e := range events {
		fmt.Fprintf(&buf, "- %s by %s at %s\n",
			e.EventType, e.Actor, e.CreatedAt.Format(time.RFC3339))
	}

	return buf.Bytes()
}

// uploadDocument sends the markdown content to the RAG Platform as a
// multipart file upload to POST {baseURL}/api/documents.
func (f *RAGFeeder) uploadDocument(
	ctx context.Context, discrepancyID string, content []byte,
) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", discrepancyID+".md")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return fmt.Errorf("write content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	url := f.baseURL + "/api/documents"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", f.apiKey)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("RAG Platform returned %d", resp.StatusCode)
	}
	return nil
}
