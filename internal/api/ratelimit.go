package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// ipBucket tracks the token bucket state for a single IP
type ipBucket struct {
	tokens   float64
	lastSeen time.Time
}

// RateLimiter implements a per-IP token bucket rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*ipBucket
	rate     float64 // tokens per second
	capacity float64 // max tokens
}

// NewRateLimiter creates a limiter allowing requestsPerMin requests per minute per IP
func NewRateLimiter(requestsPerMin int) *RateLimiter {
	rate := float64(requestsPerMin) / 60.0
	rl := &RateLimiter{
		buckets:  make(map[string]*ipBucket),
		rate:     rate,
		capacity: float64(requestsPerMin),
	}
	// Cleanup stale buckets every minute
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			rl.cleanup()
		}
	}()
	return rl
}

// Allow returns true if the request from ip is within rate limits
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		b = &ipBucket{tokens: rl.capacity, lastSeen: now}
		rl.buckets[ip] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastSeen).Seconds()
	b.tokens = min64(b.tokens+elapsed*rl.rate, rl.capacity)
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// cleanup removes buckets not seen in the last 5 minutes
func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for ip, b := range rl.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(rl.buckets, ip)
		}
	}
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// withRateLimit returns middleware that enforces per-IP rate limiting on API paths
func withRateLimit(rl *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only rate-limit the API (not static files or WebSocket)
		path := r.URL.Path
		if len(path) < 4 || path[:4] != "/api" {
			next.ServeHTTP(w, r)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		// Respect X-Forwarded-For for proxied setups (take first entry)
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			if idx := len(fwd); idx > 0 {
				for i, c := range fwd {
					if c == ',' {
						idx = i
						break
					}
				}
				ip = fwd[:idx]
			}
		}

		if !rl.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded — max 200 requests/minute")
			return
		}
		next.ServeHTTP(w, r)
	})
}
