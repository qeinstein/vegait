// Package database manages PostgreSQL and Redis connection pools,
// database migrations, batch log writes, and analytics query functions.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"

	"rate-limiter/internal/config"
)

// Container wraps the database and cache client connections.
type Container struct {
	PG    *sql.DB
	Redis *redis.Client
}

// Init initializes both PostgreSQL and Redis connection pools and runs migrations.
func Init(cfg *config.Config) (*Container, error) {
	// Initialize PostgreSQL
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("Warning: PostgreSQL not reachable at startup: %v", err)
	} else {
		log.Println("Successfully connected to PostgreSQL")
	}

	// Initialize Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DB:           0,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
		PoolSize:     50,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Redis not reachable at startup: %v", err)
	} else {
		log.Println("Successfully connected to Redis")
	}

	container := &Container{PG: db, Redis: rdb}

	if err := container.runMigrations(); err != nil {
		return nil, fmt.Errorf("failed to run database migrations: %w", err)
	}

	return container, nil
}

// Close closes all open connections.
func (c *Container) Close() {
	if c.PG != nil {
		c.PG.Close()
	}
	if c.Redis != nil {
		c.Redis.Close()
	}
}

// runMigrations creates database schemas if they do not exist.
func (c *Container) runMigrations() error {
	if err := c.PG.Ping(); err != nil {
		log.Println("Skipping database migrations as PostgreSQL is currently unavailable")
		return nil
	}

	query := `
	CREATE TABLE IF NOT EXISTS client_limits (
		client_id VARCHAR(100) PRIMARY KEY,
		rate_limit INT NOT NULL,
		window_seconds INT NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS request_logs (
		id BIGSERIAL PRIMARY KEY,
		client_id VARCHAR(100) NOT NULL,
		endpoint VARCHAR(255) NOT NULL,
		status_code INT NOT NULL,
		latency_ms INT NOT NULL,
		is_allowed BOOLEAN NOT NULL,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_logs_client_timestamp ON request_logs(client_id, timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON request_logs(timestamp);
	`

	if _, err := c.PG.Exec(query); err != nil {
		return err
	}

	var count int
	err := c.PG.QueryRow("SELECT COUNT(*) FROM client_limits").Scan(&count)
	if err == nil && count == 0 {
		seeds := `
		INSERT INTO client_limits (client_id, rate_limit, window_seconds) VALUES
		('client-alpha', 10, 10),
		('client-beta', 100, 60),
		('client-gamma', 5000, 60);
		`
		_, _ = c.PG.Exec(seeds)
		log.Println("Database tables seeded with default client limits")
	}

	return nil
}

// --- Data types ---

// LogEntry represents a single request log record.
type LogEntry struct {
	ClientID   string
	Endpoint   string
	StatusCode int
	LatencyMS  int64
	IsAllowed  bool
	Timestamp  time.Time
}

// AnalyticsSummary contains aggregate stats for the dashboard.
//
// Latency statistics (avg + percentiles) are computed over *allowed* requests
// only. Blocked (429) requests never reach the upstream, so they carry a
// latency of 0 — including them would artificially deflate the numbers.
type AnalyticsSummary struct {
	TotalRequests   int     `json:"total_requests"`
	AllowedRequests int     `json:"allowed_requests"`
	BlockedRequests int     `json:"blocked_requests"`
	AvgLatency      float64 `json:"avg_latency_ms"`
	P50Latency      float64 `json:"p50_latency_ms"`
	P95Latency      float64 `json:"p95_latency_ms"`
	P99Latency      float64 `json:"p99_latency_ms"`
}

// TrendPoint represents a single data point for time-series charts.
type TrendPoint struct {
	Label           string  `json:"label"`
	TotalRequests   int     `json:"total_requests"`
	AllowedRequests int     `json:"allowed_requests"`
	BlockedRequests int     `json:"blocked_requests"`
	AvgLatency      float64 `json:"avg_latency_ms"`
}

// TimeseriesPoint is a fine-grained (per-minute) bucket used by the live view.
// It powers the New Relic-style throughput + latency charts that update in
// near real time while a load test is running.
type TimeseriesPoint struct {
	Bucket          string  `json:"bucket"`     // RFC3339 minute bucket
	Label           string  `json:"label"`      // HH:MM for the axis
	TotalRequests   int     `json:"total_requests"`
	AllowedRequests int     `json:"allowed_requests"`
	BlockedRequests int     `json:"blocked_requests"`
	AvgLatency      float64 `json:"avg_latency_ms"`
	P95Latency      float64 `json:"p95_latency_ms"`
}

// --- Write operations ---

// InsertLogsBatch writes a slice of LogEntry records in a single transaction.
func (c *Container) InsertLogsBatch(ctx context.Context, entries []LogEntry) error {
	if len(entries) == 0 {
		return nil
	}

	tx, err := c.PG.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO request_logs (client_id, endpoint, status_code, latency_ms, is_allowed, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, e.ClientID, e.Endpoint, e.StatusCode, e.LatencyMS, e.IsAllowed, e.Timestamp); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// --- Read operations ---

// GetClientLimit retrieves a client's rate limit configuration. Falls back to defaults.
func (c *Container) GetClientLimit(ctx context.Context, clientID string) (int, int, error) {
	var rateLimit, windowSeconds int
	err := c.PG.QueryRowContext(ctx,
		"SELECT rate_limit, window_seconds FROM client_limits WHERE client_id = $1",
		clientID,
	).Scan(&rateLimit, &windowSeconds)
	if err != nil {
		if err == sql.ErrNoRows {
			return 60, 60, nil
		}
		return 0, 0, err
	}
	return rateLimit, windowSeconds, nil
}

// FetchAnalyticsSummary returns aggregate statistics for a given client and timeframe.
func (c *Container) FetchAnalyticsSummary(ctx context.Context, clientID string, days int) (AnalyticsSummary, error) {
	var s AnalyticsSummary
	since := time.Now().AddDate(0, 0, -days)

	// Latency aggregates use FILTER (WHERE is_allowed) so only real upstream
	// round-trips contribute. percentile_cont interpolates between samples.
	query := `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN NOT is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(latency_ms) FILTER (WHERE is_allowed), 0),
			COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE is_allowed), 0),
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE is_allowed), 0),
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE is_allowed), 0)
		FROM request_logs
		WHERE timestamp >= $1
	`
	var err error
	if clientID != "" {
		query += " AND client_id = $2"
		err = c.PG.QueryRowContext(ctx, query, since, clientID).Scan(
			&s.TotalRequests, &s.AllowedRequests, &s.BlockedRequests,
			&s.AvgLatency, &s.P50Latency, &s.P95Latency, &s.P99Latency)
	} else {
		err = c.PG.QueryRowContext(ctx, query, since).Scan(
			&s.TotalRequests, &s.AllowedRequests, &s.BlockedRequests,
			&s.AvgLatency, &s.P50Latency, &s.P95Latency, &s.P99Latency)
	}
	return s, err
}

// FetchAnalyticsTrend returns daily trend data for chart rendering.
func (c *Container) FetchAnalyticsTrend(ctx context.Context, clientID string, days int) ([]TrendPoint, error) {
	since := time.Now().AddDate(0, 0, -days)

	query := `
		SELECT
			TO_CHAR(timestamp, 'YYYY-MM-DD') as date_label,
			COUNT(*),
			COALESCE(SUM(CASE WHEN is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN NOT is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(latency_ms) FILTER (WHERE is_allowed), 0)
		FROM request_logs
		WHERE timestamp >= $1
	`

	var rows *sql.Rows
	var err error
	if clientID != "" {
		query += " AND client_id = $2 GROUP BY date_label ORDER BY date_label ASC"
		rows, err = c.PG.QueryContext(ctx, query, since, clientID)
	} else {
		query += " GROUP BY date_label ORDER BY date_label ASC"
		rows, err = c.PG.QueryContext(ctx, query, since)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []TrendPoint{}
	for rows.Next() {
		var pt TrendPoint
		if err := rows.Scan(&pt.Label, &pt.TotalRequests, &pt.AllowedRequests, &pt.BlockedRequests, &pt.AvgLatency); err != nil {
			return nil, err
		}
		points = append(points, pt)
	}
	return points, nil
}

// FetchLiveTimeseries returns per-minute buckets for the last `minutes` minutes.
// This is the data source for the real-time throughput and latency charts —
// during a load test the freshest buckets fill in every few seconds.
func (c *Container) FetchLiveTimeseries(ctx context.Context, clientID string, minutes int) ([]TimeseriesPoint, error) {
	if minutes <= 0 {
		minutes = 15
	}
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)

	query := `
		SELECT
			date_trunc('minute', timestamp) AS bucket,
			COUNT(*),
			COALESCE(SUM(CASE WHEN is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN NOT is_allowed THEN 1 ELSE 0 END), 0),
			COALESCE(AVG(latency_ms) FILTER (WHERE is_allowed), 0),
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms) FILTER (WHERE is_allowed), 0)
		FROM request_logs
		WHERE timestamp >= $1
	`

	var rows *sql.Rows
	var err error
	if clientID != "" {
		query += " AND client_id = $2 GROUP BY bucket ORDER BY bucket ASC"
		rows, err = c.PG.QueryContext(ctx, query, since, clientID)
	} else {
		query += " GROUP BY bucket ORDER BY bucket ASC"
		rows, err = c.PG.QueryContext(ctx, query, since)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := []TimeseriesPoint{}
	for rows.Next() {
		var pt TimeseriesPoint
		var bucket time.Time
		if err := rows.Scan(&bucket, &pt.TotalRequests, &pt.AllowedRequests,
			&pt.BlockedRequests, &pt.AvgLatency, &pt.P95Latency); err != nil {
			return nil, err
		}
		pt.Bucket = bucket.UTC().Format(time.RFC3339)
		pt.Label = bucket.Format("15:04")
		points = append(points, pt)
	}
	return points, nil
}
