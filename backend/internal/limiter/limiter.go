// Package limiter implements a distributed rate limiter using a Redis-backed
// sliding window counter with an automatic local in-memory failover.
package limiter

import (
	"context"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// Checker defines the interface for checking client rate limits.
type Checker interface {
	Allow(ctx context.Context, clientID string, limit int, windowSeconds int) (bool, error)
	IsRedisHealthy() bool
}

// slidingWindowLua is the Redis Lua script that implements the atomic sliding window algorithm.
// Uses a Sorted Set scored by the request timestamp (ms); each member is a
// UNIQUE token supplied by the caller (ARGV[4]). Using the timestamp itself as
// the member would be wrong: two requests in the same millisecond would map to
// the same member, so ZADD would overwrite rather than add and ZCARD would
// undercount — letting concurrent bursts exceed the limit.
const slidingWindowLua = `
local key = KEYS[1]
local now = tonumber(ARGV[1])
local window_ms = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local member = ARGV[4]
local clear_before = now - window_ms

redis.call('ZREMRANGEBYSCORE', key, 0, clear_before)

local current_requests = redis.call('ZCARD', key)

if current_requests < limit then
    redis.call('ZADD', key, now, member)
    redis.call('EXPIRE', key, math.ceil(window_ms / 1000))
    return 1
else
    return 0
end
`

// Manager orchestrates rate limiting with high-availability failover.
type Manager struct {
	redisClient    *redis.Client
	luaSHA         string
	redisHealthy   int32  // atomic: 1 = healthy, 0 = unhealthy
	consecFailures int32
	memberSeq      uint64 // atomic: guarantees unique sorted-set members

	// Local fallback: maps clientID -> *localLimiterEntry
	localLimiters sync.Map

	healthCheckInterval time.Duration
	maxFailures         int32
}

type localLimiterEntry struct {
	limiter       *rate.Limiter
	limit         int
	windowSeconds int
	lastAccessed  time.Time
}

// New creates a Manager with automatic Redis failover.
func New(rdb *redis.Client) *Manager {
	mgr := &Manager{
		redisClient:         rdb,
		redisHealthy:        1,
		healthCheckInterval: 5 * time.Second,
		maxFailures:         3,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sha, err := rdb.ScriptLoad(ctx, slidingWindowLua).Result()
	if err != nil {
		log.Printf("Warning: Failed to pre-load Lua script in Redis: %v. Will fall back to EVAL.", err)
	} else {
		mgr.luaSHA = sha
		log.Printf("Loaded rate limiter Lua script into Redis (SHA: %s)", sha)
	}

	go mgr.startLocalLimiterCleanupLoop()

	return mgr
}

// IsRedisHealthy returns current Redis health status.
func (m *Manager) IsRedisHealthy() bool {
	return atomic.LoadInt32(&m.redisHealthy) == 1
}

// Allow checks if a request is permitted. Falls back to local limiter on Redis failure.
func (m *Manager) Allow(ctx context.Context, clientID string, limit int, windowSeconds int) (bool, error) {
	if m.IsRedisHealthy() {
		allowed, err := m.allowRedis(ctx, clientID, limit, windowSeconds)
		if err == nil {
			atomic.StoreInt32(&m.consecFailures, 0)
			return allowed, nil
		}
		log.Printf("Redis error for client %s: %v. Incrementing failure count.", clientID, err)
		m.handleRedisFailure()
	}

	return m.allowLocal(clientID, limit, windowSeconds), nil
}

func (m *Manager) allowRedis(ctx context.Context, clientID string, limit int, windowSeconds int) (bool, error) {
	key := "limiter:" + clientID
	nowNS := time.Now().UnixNano()
	nowMS := nowNS / int64(time.Millisecond)
	windowMS := int64(windowSeconds * 1000)

	// A unique member per request: nanosecond timestamp + a process-wide atomic
	// counter, so even requests in the same nanosecond never collide.
	member := strconv.FormatInt(nowNS, 10) + "-" + strconv.FormatUint(atomic.AddUint64(&m.memberSeq, 1), 10)

	args := []interface{}{nowMS, windowMS, limit, member}

	var result interface{}
	var err error

	if m.luaSHA != "" {
		result, err = m.redisClient.EvalSha(ctx, m.luaSHA, []string{key}, args...).Result()
		if err != nil && isNoScriptError(err) {
			result, err = m.redisClient.Eval(ctx, slidingWindowLua, []string{key}, args...).Result()
		}
	} else {
		result, err = m.redisClient.Eval(ctx, slidingWindowLua, []string{key}, args...).Result()
	}

	if err != nil {
		return false, err
	}

	val, ok := result.(int64)
	if !ok {
		return false, nil
	}
	return val == 1, nil
}

func (m *Manager) allowLocal(clientID string, limit int, windowSeconds int) bool {
	now := time.Now()
	val, exists := m.localLimiters.Load(clientID)
	var entry *localLimiterEntry

	if exists {
		entry = val.(*localLimiterEntry)
		if entry.limit != limit || entry.windowSeconds != windowSeconds {
			entry = m.createLocalEntry(limit, windowSeconds)
			m.localLimiters.Store(clientID, entry)
		}
	} else {
		entry = m.createLocalEntry(limit, windowSeconds)
		m.localLimiters.Store(clientID, entry)
	}

	entry.lastAccessed = now
	return entry.limiter.Allow()
}

func (m *Manager) createLocalEntry(limit int, windowSeconds int) *localLimiterEntry {
	tokensPerSec := float64(limit) / float64(windowSeconds)
	return &localLimiterEntry{
		limiter:       rate.NewLimiter(rate.Limit(tokensPerSec), limit),
		limit:         limit,
		windowSeconds: windowSeconds,
		lastAccessed:  time.Now(),
	}
}

// HandleRedisFailure is exported so tests can force the circuit breaker open.
func (m *Manager) HandleRedisFailure() {
	m.handleRedisFailure()
}

func (m *Manager) handleRedisFailure() {
	fails := atomic.AddInt32(&m.consecFailures, 1)
	if fails >= m.maxFailures && m.IsRedisHealthy() {
		if atomic.CompareAndSwapInt32(&m.redisHealthy, 1, 0) {
			log.Printf("CRITICAL: Redis failed %d times. Entering FAIL-SAFE mode.", fails)
			go m.startRedisRecoveryHealthCheck()
		}
	}
}

func (m *Manager) startRedisRecoveryHealthCheck() {
	ticker := time.NewTicker(m.healthCheckInterval)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := m.redisClient.Ping(ctx).Err()
		cancel()

		if err == nil {
			log.Println("RECOVERY: Redis reachable. Exiting FAIL-SAFE mode.")
			atomic.StoreInt32(&m.redisHealthy, 1)
			atomic.StoreInt32(&m.consecFailures, 0)

			if m.luaSHA == "" {
				ctxReload, cancelReload := context.WithTimeout(context.Background(), 2*time.Second)
				sha, reloadErr := m.redisClient.ScriptLoad(ctxReload, slidingWindowLua).Result()
				cancelReload()
				if reloadErr == nil {
					m.luaSHA = sha
				}
			}
			return
		}
		log.Printf("Recovery health check: Redis still unreachable: %v", err)
	}
}

func (m *Manager) startLocalLimiterCleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		now := time.Now()
		m.localLimiters.Range(func(key, value interface{}) bool {
			entry := value.(*localLimiterEntry)
			inactiveDuration := time.Duration(entry.windowSeconds) * 5 * time.Second
			if inactiveDuration < 5*time.Minute {
				inactiveDuration = 5 * time.Minute
			}
			if now.Sub(entry.lastAccessed) > inactiveDuration {
				m.localLimiters.Delete(key)
			}
			return true
		})
	}
}

func isNoScriptError(err error) bool {
	return err != nil && err.Error() == "NOSCRIPT No matching script. Please use EVAL."
}
