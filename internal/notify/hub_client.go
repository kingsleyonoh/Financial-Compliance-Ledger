// Package notify provides the Notification Hub REST client.
// It sends escalation events to the Event-Driven Notification Hub
// with retry logic and exponential backoff.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
)

// NotificationSender is the interface for sending hub events.
// Implemented by HubClient; can be mocked in tests.
type NotificationSender interface {
	SendEvent(ctx context.Context, event HubEvent) (*HubResponse, error)
}

// HubEvent is the payload sent to the Notification Hub.
type HubEvent struct {
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Payload   map[string]interface{} `json:"payload"`
}

// HubResponse is the response from the Notification Hub.
type HubResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

// defaultBackoffSchedule is the exponential backoff: 1s, 4s, 16s.
var defaultBackoffSchedule = []time.Duration{
	1 * time.Second,
	4 * time.Second,
	16 * time.Second,
}

// HubClient sends events to the Event-Driven Notification Hub.
type HubClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	enabled    bool
	maxRetries int
	backoff    []time.Duration
	logger     zerolog.Logger
}

// NewHubClient creates a HubClient from configuration.
func NewHubClient(cfg *config.Config, logger zerolog.Logger) *HubClient {
	return NewHubClientWithBackoff(cfg, logger, defaultBackoffSchedule)
}

// NewHubClientWithBackoff creates a HubClient with custom backoff
// schedule. Used for testing with shorter delays.
func NewHubClientWithBackoff(
	cfg *config.Config, logger zerolog.Logger,
	backoff []time.Duration,
) *HubClient {
	return &HubClient{
		baseURL: cfg.NotificationHubURL,
		apiKey:  cfg.NotificationHubAPIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		enabled:    cfg.NotificationHubEnabled,
		maxRetries: cfg.MaxNotificationRetries,
		backoff:    backoff,
		logger: logger.With().
			Str("component", "hub-client").Logger(),
	}
}

// SendEvent sends a HubEvent to the Notification Hub.
// If the hub is disabled, returns immediately with status "skipped".
// Retries with exponential backoff on server errors (5xx).
// Client errors (4xx) are not retried — the response is returned.
func (c *HubClient) SendEvent(
	ctx context.Context, event HubEvent,
) (*HubResponse, error) {
	if !c.enabled {
		c.logger.Debug().
			Str("event_type", event.EventType).
			Msg("notification hub disabled, skipping")
		return &HubResponse{Status: "skipped"}, nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("hub_client.SendEvent: marshal: %w", err)
	}

	maxAttempts := c.maxRetries
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := c.backoffDelay(attempt - 1)
			c.logger.Debug().
				Int("attempt", attempt+1).
				Dur("delay", delay).
				Msg("retrying hub request")

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf(
		"hub_client.SendEvent: retries exhausted after %d attempts: %w",
		maxAttempts, lastErr)
}

// doRequest makes a single HTTP POST to the hub and parses the
// response. Returns an error for server errors (5xx) to trigger
// retry. Returns the response (no error) for client errors (4xx).
func (c *HubClient) doRequest(
	ctx context.Context, body []byte,
) (*HubResponse, error) {
	url := c.baseURL + "/api/events"
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Server error — return error to trigger retry
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("server error: %d", resp.StatusCode)
	}

	var hubResp HubResponse
	if err := json.Unmarshal(respBody, &hubResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &hubResp, nil
}

// backoffDelay returns the delay for the given retry index.
func (c *HubClient) backoffDelay(retryIndex int) time.Duration {
	if retryIndex < len(c.backoff) {
		return c.backoff[retryIndex]
	}
	return c.backoff[len(c.backoff)-1]
}
