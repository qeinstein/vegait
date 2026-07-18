// Package middleware provides production HTTP middleware: request IDs,
// CORS, panic recovery, access logging, and client ID enforcement.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

type requestIDKey struct{}

// RequestIDFromContext retrieves the request ID from a context.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return "unknown"
}

// RequestID generates a unique request ID and injects it into context + headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CORS adds cross-origin resource sharing headers for the React dashboard.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Client-ID, X-Target-URL, X-Request-ID")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-RateLimit-Remaining, X-RateLimit-Limit")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Recovery catches panics and returns a 500 instead of crashing.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := RequestIDFromContext(r.Context())
				log.Printf("[PANIC] request_id=%s path=%s error=%v", reqID, r.URL.Path, rec)
				http.Error(w, `{"error": "internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// AccessLog logs every request with method, path, status, and duration.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		reqID := RequestIDFromContext(r.Context())
		log.Printf("[HTTP] request_id=%s method=%s path=%s status=%d duration=%s remote=%s",
			reqID, r.Method, r.URL.Path, rec.status, time.Since(start), remoteIP(r))
	})
}

// ClientID rejects proxy requests that are missing the X-Client-ID header.
func ClientID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/proxy") {
			if r.Header.Get("X-Client-ID") == "" {
				http.Error(w, `{"error": "X-Client-ID header is required"}`, http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Chain applies middlewares in order (outermost first).
func Chain(handler http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}

// --- internals ---

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func remoteIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}
