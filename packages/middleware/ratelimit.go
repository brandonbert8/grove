package middleware

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brandonbert8/grove/packages/router"
)

// RateLimit throttles requests per client IP with a token bucket:
// burst tokens capacity, refilled at rps tokens/second. Excess gets a
// JSON 429 with an honest Retry-After. Non-positive rps disables limiting.
//
// The client key is the TCP peer (RemoteAddr), which cannot be spoofed.
// Behind a trusted proxy where every connection arrives from one IP,
// opt into X-Forwarded-For with RateLimitWithConfig{TrustProxy: true}.
// This is process-local — use a shared store (Redis, gateway) for
// replicas; past the bucket cap the limiter fails open (allows).
func RateLimit(rps, burst int) router.Middleware {
	return RateLimitWithConfig(RateLimitConfig{RPS: rps, Burst: burst})
}

// defaultMaxClients bounds bucket memory under rotating-IP floods.
const defaultMaxClients = 8192

// RateLimitConfig tunes RateLimitWithConfig.
type RateLimitConfig struct {
	// RPS refills tokens/second; Burst is bucket capacity. Non-positive
	// values disable limiting (passthrough).
	RPS, Burst int
	// TrustProxy keys by the first X-Forwarded-For entry instead of the
	// TCP peer. Enable only behind a proxy you control: clients can
	// spoof XFF to dodge limits when directly exposed.
	TrustProxy bool
	// MaxClients caps tracked buckets (0 means defaultMaxClients). Past
	// the cap, unknown clients pass untracked rather than growing memory.
	MaxClients int
}

// RateLimitWithConfig throttles like RateLimit with explicit options.
func RateLimitWithConfig(cfg RateLimitConfig) router.Middleware {
	if cfg.RPS <= 0 || cfg.Burst <= 0 {
		return func(next router.HandlerFunc) router.HandlerFunc { return next }
	}
	max := cfg.MaxClients
	if max <= 0 {
		max = defaultMaxClients
	}
	l := &ipLimiter{
		clients:    map[string]*bucket{},
		rps:        float64(cfg.RPS),
		burst:      float64(cfg.Burst),
		maxClients: max,
	}
	keyOf := remoteIP
	if cfg.TrustProxy {
		keyOf = clientIP
	}
	return func(next router.HandlerFunc) router.HandlerFunc {
		return func(c router.Context) error {
			wait := l.take(keyOf(c.Request()))
			if wait <= 0 {
				return next(c)
			}
			// Honest Retry-After: round the token deficit up, minimum 1s.
			retry := int(math.Ceil(wait.Seconds()))
			if retry < 1 {
				retry = 1
			}
			h := c.ResponseWriter().Header()
			h.Set("Retry-After", strconv.Itoa(retry))
			return c.JSON(http.StatusTooManyRequests, map[string]any{
				"error":  "rate limit exceeded",
				"status": http.StatusTooManyRequests,
			})
		}
	}
}

// bucket is one client's token bucket.
type bucket struct {
	tokens float64
	last   time.Time
}

// ipLimiter holds per-IP buckets. All methods take l.mu.
type ipLimiter struct {
	mu         sync.Mutex
	clients    map[string]*bucket
	rps        float64
	burst      float64
	maxClients int
}

// take consumes one token, returning how long to wait when empty.
// Unknown clients past the bucket cap pass untracked (fail-open)
// instead of growing memory under rotating-IP floods.
func (l *ipLimiter) take(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.clients[key]
	if !ok {
		if len(l.clients) >= l.maxClients {
			return 0
		}
		b = &bucket{tokens: l.burst, last: now}
		l.clients[key] = b
		if len(l.clients) > 1024 {
			l.sweepLocked(now)
		}
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rps
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	deficit := 1 - b.tokens
	return time.Duration(deficit / l.rps * float64(time.Second))
}

// sweepLocked drops buckets idle over a minute. Callers hold l.mu.
func (l *ipLimiter) sweepLocked(now time.Time) {
	for k, b := range l.clients {
		if now.Sub(b.last) > time.Minute {
			delete(l.clients, k)
		}
	}
}

// clientIP prefers X-Forwarded-For, then RemoteAddr host. Use only
// with TrustProxy: XFF is client-controlled when directly exposed.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip, _, _ := strings.Cut(xff, ","); strings.TrimSpace(ip) != "" {
			return strings.TrimSpace(ip)
		}
	}
	return remoteIP(r)
}

// remoteIP keys by the TCP peer, which cannot be spoofed.
func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
