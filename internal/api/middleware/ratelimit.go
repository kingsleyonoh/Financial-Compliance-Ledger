package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kingsleyonoh/Financial-Compliance-Ledger/internal/api/handlers"
)

// ipRecord tracks request count and window start for a single IP.
type ipRecord struct {
	count     int
	windowEnd time.Time
}

// rateLimiter stores per-IP request counts with a sliding window.
type rateLimiter struct {
	mu                sync.Mutex
	records           map[string]*ipRecord
	requestsPerMinute int
}

// NewRateLimiter returns middleware that limits requests per IP address.
// requestsPerMinute specifies the maximum allowed requests within a
// rolling 60-second window per IP.
func NewRateLimiter(requestsPerMinute int) func(next http.Handler) http.Handler {
	rl := &rateLimiter{
		records:           make(map[string]*ipRecord),
		requestsPerMinute: requestsPerMinute,
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := rl.extractIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "60")
				handlers.RespondError(w, http.StatusTooManyRequests,
					"RATE_LIMIT_EXCEEDED", "Too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// allow checks whether the given IP is within its rate limit and
// increments the counter. Returns true if the request is allowed.
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	rec, exists := rl.records[ip]

	if !exists || now.After(rec.windowEnd) {
		// New window
		rl.records[ip] = &ipRecord{
			count:     1,
			windowEnd: now.Add(time.Minute),
		}
		return true
	}

	if rec.count >= rl.requestsPerMinute {
		return false
	}

	rec.count++
	return true
}

// extractIP returns the client IP from X-Forwarded-For or RemoteAddr.
func (rl *rateLimiter) extractIP(r *http.Request) string {
	// Check X-Forwarded-For first (proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
