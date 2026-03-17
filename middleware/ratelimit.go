package middleware

import (
	"log/slog"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig controls rate limiting.
type RateLimitConfig struct {
	GlobalPerSec float64       // max requests/sec globally (0 = unlimited)
	PerKeyPerSec float64       // max requests/sec per key (0 = unlimited)
	PerKeyBurst  int           // burst allowance per key
	KeyLimiterTTL time.Duration // cleanup interval for stale per-key limiters
}

// DefaultRateLimitConfig returns sensible defaults.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		GlobalPerSec:  100,
		PerKeyPerSec:  5,
		PerKeyBurst:   3,
		KeyLimiterTTL: 5 * time.Minute,
	}
}

// RateLimiter enforces global and per-key rate limits.
type RateLimiter struct {
	Config        RateLimitConfig
	globalLimiter *rate.Limiter
	keyLimiters   map[string]*keyLimiterEntry
	mu            sync.Mutex
	// OnRejected is called when a request is rate-limited.
	// The key argument is the rate limit key (e.g., IP address, subject ID).
	OnRejected func(key string)
}

type keyLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter. Returns nil if both limits are 0.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.GlobalPerSec <= 0 && cfg.PerKeyPerSec <= 0 {
		return nil
	}
	rl := &RateLimiter{
		Config:      cfg,
		keyLimiters: make(map[string]*keyLimiterEntry),
	}
	if cfg.GlobalPerSec > 0 {
		rl.globalLimiter = rate.NewLimiter(rate.Limit(cfg.GlobalPerSec), int(cfg.GlobalPerSec))
	}
	if cfg.KeyLimiterTTL > 0 {
		go rl.cleanupKeyLimiters()
	}
	return rl
}

func (rl *RateLimiter) getKeyLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	entry, ok := rl.keyLimiters[key]
	if !ok {
		entry = &keyLimiterEntry{
			limiter: rate.NewLimiter(rate.Limit(rl.Config.PerKeyPerSec), rl.Config.PerKeyBurst),
		}
		rl.keyLimiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter
}

func (rl *RateLimiter) cleanupKeyLimiters() {
	ticker := time.NewTicker(rl.Config.KeyLimiterTTL)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.Config.KeyLimiterTTL)
		for key, entry := range rl.keyLimiters {
			if entry.lastSeen.Before(cutoff) {
				delete(rl.keyLimiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks both global and per-key rate limits. Returns false if rejected.
func (rl *RateLimiter) Allow(key string) bool {
	if rl == nil {
		return true
	}
	if rl.globalLimiter != nil && !rl.globalLimiter.Allow() {
		return false
	}
	if rl.Config.PerKeyPerSec > 0 && !rl.getKeyLimiter(key).Allow() {
		return false
	}
	return true
}

// Middleware returns an HTTP middleware that enforces rate limits.
// keyFunc extracts the rate limit key from the request. If nil, defaults to ClientIP.
func (rl *RateLimiter) Middleware(keyFunc func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if rl == nil {
			return next
		}
		if keyFunc == nil {
			keyFunc = ClientIP
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFunc(r)
			if !rl.Allow(key) {
				if rl.OnRejected != nil {
					rl.OnRejected(key)
				}
				slog.Warn("Rate limited request", "component", "ratelimit", "key", key, "path", r.URL.Path)
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
