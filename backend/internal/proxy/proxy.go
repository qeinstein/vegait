// Package proxy implements the reverse proxy gateway that intercepts requests,
// checks rate limits, forwards to third-party APIs, and logs telemetry.
package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"rate-limiter/internal/config"
	"rate-limiter/internal/database"
	"rate-limiter/internal/httputil"
	"rate-limiter/internal/limiter"
	"rate-limiter/internal/logger"
)

// Handler is the core gateway handler.
type Handler struct {
	db      *database.Container
	limiter *limiter.Manager
	logger  *logger.Async
	cfg     *config.Config
	client  *http.Client
}

// NewHandler creates a Handler with a pre-configured outbound HTTP client.
func NewHandler(db *database.Container, lim *limiter.Manager, lg *logger.Async, cfg *config.Config) *Handler {
	return &Handler{
		db:      db,
		limiter: lim,
		logger:  lg,
		cfg:     cfg,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// ServeHTTP implements the gateway flow:
//  1. Extract client ID + target URL
//  2. Fetch client rate limit from DB
//  3. Check rate limit
//  4. Forward or reject
//  5. Async log
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	clientID := r.Header.Get("X-Client-ID")
	targetURL := r.Header.Get("X-Target-URL")

	if clientID == "" {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "X-Client-ID header is required"})
		return
	}
	if targetURL == "" {
		targetURL = h.cfg.MockAPIURL + r.URL.Path
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rateLimit, windowSecs, err := h.db.GetClientLimit(ctx, clientID)
	if err != nil {
		log.Printf("Error fetching client limit for %s: %v. Using defaults.", clientID, err)
		rateLimit = h.cfg.DefaultRateLimit
		windowSecs = h.cfg.DefaultWindowSecs
	}

	limiterCtx, limiterCancel := context.WithTimeout(r.Context(), 5*time.Millisecond)
	defer limiterCancel()

	allowed, limiterErr := h.limiter.Allow(limiterCtx, clientID, rateLimit, windowSecs)
	if limiterErr != nil {
		log.Printf("Limiter error for client %s: %v. Failing open.", clientID, limiterErr)
		allowed = true
	}

	now := time.Now()

	if !allowed {
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rateLimit))
		w.Header().Set("X-RateLimit-Window", fmt.Sprintf("%ds", windowSecs))
		w.Header().Set("Retry-After", fmt.Sprintf("%d", windowSecs))
		httputil.WriteJSON(w, http.StatusTooManyRequests, map[string]string{
			"error":     "rate limit exceeded",
			"client_id": clientID,
			"limit":     fmt.Sprintf("%d requests per %d seconds", rateLimit, windowSecs),
		})

		h.logger.Log(database.LogEntry{
			ClientID: clientID, Endpoint: targetURL,
			StatusCode: http.StatusTooManyRequests,
			LatencyMS: 0, IsAllowed: false, Timestamp: now,
		})
		return
	}

	proxyStart := time.Now()
	statusCode, respBody, respHeaders, proxyErr := h.forwardRequest(r, targetURL)
	latency := time.Since(proxyStart).Milliseconds()

	h.logger.Log(database.LogEntry{
		ClientID: clientID, Endpoint: targetURL,
		StatusCode: statusCode,
		LatencyMS: latency, IsAllowed: true, Timestamp: now,
	})

	if proxyErr != nil {
		log.Printf("Proxy error for client %s -> %s: %v", clientID, targetURL, proxyErr)
		httputil.WriteJSON(w, http.StatusBadGateway, map[string]string{
			"error": "upstream request failed", "details": proxyErr.Error(),
		})
		return
	}

	for key, values := range respHeaders {
		for _, val := range values {
			w.Header().Add(key, val)
		}
	}
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rateLimit))
	w.Header().Set("X-RateLimit-Window", fmt.Sprintf("%ds", windowSecs))
	w.Header().Set("X-Upstream-Latency-Ms", fmt.Sprintf("%d", latency))
	w.WriteHeader(statusCode)
	w.Write(respBody)
}

func (h *Handler) forwardRequest(r *http.Request, targetURL string) (int, []byte, http.Header, error) {
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		return http.StatusBadGateway, nil, nil, fmt.Errorf("failed to create outbound request: %w", err)
	}

	for _, hdr := range []string{"Content-Type", "Accept", "Authorization", "User-Agent"} {
		if v := r.Header.Get(hdr); v != "" {
			outReq.Header.Set(hdr, v)
		}
	}

	resp, err := h.client.Do(outReq)
	if err != nil {
		return http.StatusBadGateway, nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return resp.StatusCode, nil, resp.Header, fmt.Errorf("failed to read upstream body: %w", err)
	}
	return resp.StatusCode, body, resp.Header, nil
}
