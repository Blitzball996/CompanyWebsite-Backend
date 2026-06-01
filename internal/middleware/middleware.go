package middleware

import (
	"net/http"
	"sync"
	"time"
)

// CORS allows the configured website origins to POST to the collect endpoint.
func CORS(allowed []string) func(http.Handler) http.Handler {
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		set[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (set[origin] || set["*"]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit is a simple per-IP token bucket (refills over time).
type bucket struct {
	tokens float64
	last   time.Time
}

func RateLimit(perMinute float64, burst float64) func(http.Handler) http.Handler {
	var mu sync.Mutex
	buckets := map[string]*bucket{}
	rate := perMinute / 60.0 // tokens per second
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			mu.Lock()
			b, ok := buckets[ip]
			now := time.Now()
			if !ok {
				b = &bucket{tokens: burst, last: now}
				buckets[ip] = b
			}
			b.tokens += now.Sub(b.last).Seconds() * rate
			if b.tokens > burst {
				b.tokens = burst
			}
			b.last = now
			if b.tokens < 1 {
				mu.Unlock()
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			b.tokens--
			mu.Unlock()
			next.ServeHTTP(w, r)
		})
	}
}

func clientIP(r *http.Request) string {
	// Behind Cloudflare the real visitor IP is in CF-Connecting-IP; prefer it so
	// rate-limiting and geolocation see the real client, not Cloudflare's edge.
	if cf := r.Header.Get("CF-Connecting-IP"); cf != "" {
		return cf
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

// ClientIP is exported for handlers that need the caller IP (for visitor hashing).
func ClientIP(r *http.Request) string { return clientIP(r) }
