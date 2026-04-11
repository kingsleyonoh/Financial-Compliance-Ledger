package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/config"
	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/notify"
)

func testLogger(t *testing.T) zerolog.Logger {
	return zerolog.New(zerolog.NewTestWriter(t)).With().
		Timestamp().Logger()
}

func TestHubClient_SendEvent_Success(t *testing.T) {
	var received notify.HubEvent
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/api/events", r.URL.Path)
			assert.Equal(t, "test-api-key", r.Header.Get("X-API-Key"))
			assert.Equal(t, "application/json",
				r.Header.Get("Content-Type"))

			err := json.NewDecoder(r.Body).Decode(&received)
			require.NoError(t, err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "accepted",
				"id":     "hub-event-123",
			})
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-api-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 3,
	}

	client := notify.NewHubClient(cfg, testLogger(t))

	event := notify.HubEvent{
		EventType: "compliance.escalation_triggered",
		TenantID:  "tenant-123",
		Payload: map[string]interface{}{
			"discrepancy_id": "disc-456",
			"severity":       "high",
		},
	}

	resp, err := client.SendEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.Equal(t, "hub-event-123", resp.ID)

	// Verify the event was sent correctly
	assert.Equal(t, "compliance.escalation_triggered", received.EventType)
	assert.Equal(t, "tenant-123", received.TenantID)
}

func TestHubClient_SendEvent_DisabledSkips(t *testing.T) {
	cfg := &config.Config{
		NotificationHubURL:     "http://should-not-be-called",
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: false,
		MaxNotificationRetries: 3,
	}

	client := notify.NewHubClient(cfg, testLogger(t))

	event := notify.HubEvent{
		EventType: "compliance.escalation_triggered",
		TenantID:  "tenant-123",
		Payload:   map[string]interface{}{},
	}

	resp, err := client.SendEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "skipped", resp.Status)
}

func TestHubClient_SendEvent_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "rejected",
				"id":     "",
			})
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 1,
	}

	client := notify.NewHubClient(cfg, testLogger(t))

	event := notify.HubEvent{
		EventType: "compliance.escalation_triggered",
		TenantID:  "tenant-123",
		Payload:   map[string]interface{}{},
	}

	resp, err := client.SendEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "rejected", resp.Status)
}

func TestHubClient_SendEvent_RetriesOnServerError(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&callCount, 1)
			if count < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			// Third call succeeds
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "accepted",
				"id":     "retry-success",
			})
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 3,
	}

	client := notify.NewHubClientWithBackoff(
		cfg, testLogger(t), []time.Duration{
			10 * time.Millisecond,
			20 * time.Millisecond,
			40 * time.Millisecond,
		})

	event := notify.HubEvent{
		EventType: "test.retry",
		TenantID:  "tenant-123",
		Payload:   map[string]interface{}{},
	}

	resp, err := client.SendEvent(context.Background(), event)
	require.NoError(t, err)
	assert.Equal(t, "accepted", resp.Status)
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount))
}

func TestHubClient_SendEvent_ExhaustsRetries(t *testing.T) {
	var callCount int32

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&callCount, 1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 3,
	}

	client := notify.NewHubClientWithBackoff(
		cfg, testLogger(t), []time.Duration{
			10 * time.Millisecond,
			20 * time.Millisecond,
			40 * time.Millisecond,
		})

	event := notify.HubEvent{
		EventType: "test.exhaust",
		TenantID:  "tenant-123",
		Payload:   map[string]interface{}{},
	}

	resp, err := client.SendEvent(context.Background(), event)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "retries exhausted")
	assert.Equal(t, int32(3), atomic.LoadInt32(&callCount))
}

func TestHubClient_SendEvent_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
	defer srv.Close()

	cfg := &config.Config{
		NotificationHubURL:     srv.URL,
		NotificationHubAPIKey:  "test-key",
		NotificationHubEnabled: true,
		MaxNotificationRetries: 1,
	}

	client := notify.NewHubClient(cfg, testLogger(t))

	event := notify.HubEvent{
		EventType: "test.cancel",
		TenantID:  "tenant-123",
		Payload:   map[string]interface{}{},
	}

	ctx, cancel := context.WithTimeout(context.Background(),
		100*time.Millisecond)
	defer cancel()

	_, err := client.SendEvent(ctx, event)
	require.Error(t, err)
}

func TestHubClient_ImplementsNotificationSender(t *testing.T) {
	cfg := &config.Config{
		NotificationHubEnabled: false,
	}
	client := notify.NewHubClient(cfg, testLogger(t))

	// Compile-time interface check
	var _ notify.NotificationSender = client
}
