// Package handlers provides REST API endpoints for the admin dashboard,
// health checks, client management, and logger statistics.
package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"rate-limiter/internal/config"
	"rate-limiter/internal/database"
	"rate-limiter/internal/httputil"
	"rate-limiter/internal/limiter"
	"rate-limiter/internal/logger"
)

// Admin serves dashboard API endpoints and health checks.
type Admin struct {
	db      *database.Container
	limiter *limiter.Manager
	logger  *logger.Async
	cfg     *config.Config
}

// NewAdmin constructs an Admin handler with all dependencies injected.
func NewAdmin(db *database.Container, lim *limiter.Manager, lg *logger.Async, cfg *config.Config) *Admin {
	return &Admin{db: db, limiter: lim, logger: lg, cfg: cfg}
}

// RegisterRoutes maps admin paths to their handler methods.
func (a *Admin) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", a.HealthCheck)
	mux.HandleFunc("/health/deep", a.DeepHealthCheck)
	mux.HandleFunc("/api/analytics/summary", a.GetAnalyticsSummary)
	mux.HandleFunc("/api/analytics/trend", a.GetAnalyticsTrend)
	mux.HandleFunc("/api/analytics/live", a.GetLiveTimeseries)
	mux.HandleFunc("/api/clients", a.ListClients)
	mux.HandleFunc("/api/clients/upsert", a.UpsertClient)
	mux.HandleFunc("/api/logger/stats", a.LoggerStats)
}

// HealthCheck is a lightweight liveness probe.
func (a *Admin) HealthCheck(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "ok",
		"redis_healthy": a.limiter.IsRedisHealthy(),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	})
}

// DeepHealthCheck verifies connectivity to PostgreSQL and Redis.
func (a *Admin) DeepHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	pgOK := a.db.PG.PingContext(ctx) == nil
	redisOK := a.limiter.IsRedisHealthy()

	status := "healthy"
	httpStatus := http.StatusOK
	if !pgOK || !redisOK {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	httputil.WriteJSON(w, httpStatus, map[string]interface{}{
		"status":    status,
		"postgres":  pgOK,
		"redis":     redisOK,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// GetAnalyticsSummary returns aggregate stats.
func (a *Admin) GetAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	days := httputil.QueryInt(r, "days", 30)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	summary, err := a.db.FetchAnalyticsSummary(ctx, clientID, days)
	if err != nil {
		log.Printf("Error fetching analytics summary: %v", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch analytics"})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, summary)
}

// GetAnalyticsTrend returns daily trend data for chart rendering.
func (a *Admin) GetAnalyticsTrend(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	days := httputil.QueryInt(r, "days", 30)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trend, err := a.db.FetchAnalyticsTrend(ctx, clientID, days)
	if err != nil {
		log.Printf("Error fetching analytics trend: %v", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch trend data"})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, trend)
}

// GetLiveTimeseries returns per-minute buckets for the real-time dashboard view.
func (a *Admin) GetLiveTimeseries(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	minutes := httputil.QueryInt(r, "minutes", 15)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	series, err := a.db.FetchLiveTimeseries(ctx, clientID, minutes)
	if err != nil {
		log.Printf("Error fetching live timeseries: %v", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch live data"})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, series)
}

// ClientInfo represents client limit config returned by the API.
type ClientInfo struct {
	ClientID      string `json:"client_id"`
	RateLimit     int    `json:"rate_limit"`
	WindowSeconds int    `json:"window_seconds"`
}

// ListClients returns all configured client limits.
func (a *Admin) ListClients(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	rows, err := a.db.PG.QueryContext(ctx,
		"SELECT client_id, rate_limit, window_seconds FROM client_limits ORDER BY client_id ASC")
	if err != nil {
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query clients"})
		return
	}
	defer rows.Close()

	clients := []ClientInfo{}
	for rows.Next() {
		var c ClientInfo
		if err := rows.Scan(&c.ClientID, &c.RateLimit, &c.WindowSeconds); err != nil {
			continue
		}
		clients = append(clients, c)
	}
	httputil.WriteJSON(w, http.StatusOK, clients)
}

// UpsertClientRequest is the request body for creating/updating a client limit.
type UpsertClientRequest struct {
	ClientID      string `json:"client_id"`
	RateLimit     int    `json:"rate_limit"`
	WindowSeconds int    `json:"window_seconds"`
}

// UpsertClient creates or updates a client's rate limit configuration.
func (a *Admin) UpsertClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httputil.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req UpsertClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.ClientID == "" || req.RateLimit <= 0 || req.WindowSeconds <= 0 {
		httputil.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"error": "client_id, rate_limit (>0), and window_seconds (>0) are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	query := `
		INSERT INTO client_limits (client_id, rate_limit, window_seconds, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (client_id) DO UPDATE SET
			rate_limit = EXCLUDED.rate_limit,
			window_seconds = EXCLUDED.window_seconds,
			updated_at = NOW()
	`
	if _, err := a.db.PG.ExecContext(ctx, query, req.ClientID, req.RateLimit, req.WindowSeconds); err != nil {
		log.Printf("Error upserting client: %v", err)
		httputil.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to upsert client"})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "client_id": req.ClientID})
}

// LoggerStats exposes the async logger's internal metrics.
func (a *Admin) LoggerStats(w http.ResponseWriter, r *http.Request) {
	logged, dropped, flushErrs := a.logger.Stats()
	httputil.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"total_logged":       logged,
		"total_dropped":      dropped,
		"total_flush_errors": flushErrs,
	})
}
