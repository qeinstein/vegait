package tests

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"rate-limiter/internal/limiter"
)

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// Unit Tests

func TestAllowRedis_BasicLimit(t *testing.T) {
	rdb := newTestRedisClient(t)
	mgr := limiter.New(rdb)
	ctx := context.Background()
	clientID := "test-basic-limit"

	rdb.Del(ctx, "limiter:"+clientID)
	defer rdb.Del(ctx, "limiter:"+clientID)

	limit := 5
	windowSecs := 10

	for i := 0; i < limit; i++ {
		allowed, err := mgr.Allow(ctx, clientID, limit, windowSecs)
		if err != nil {
			t.Fatalf("Unexpected error on request %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("Request %d should have been allowed", i+1)
		}
	}

	allowed, err := mgr.Allow(ctx, clientID, limit, windowSecs)
	if err != nil {
		t.Fatalf("Unexpected error on over-limit request: %v", err)
	}
	if allowed {
		t.Fatal("Request should have been blocked after exceeding the limit")
	}
}

func TestAllowRedis_WindowExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping time-dependent test in short mode")
	}

	rdb := newTestRedisClient(t)
	mgr := limiter.New(rdb)
	ctx := context.Background()
	clientID := "test-window-expiry"

	rdb.Del(ctx, "limiter:"+clientID)
	defer rdb.Del(ctx, "limiter:"+clientID)

	limit := 3
	windowSecs := 2

	for i := 0; i < limit; i++ {
		allowed, _ := mgr.Allow(ctx, clientID, limit, windowSecs)
		if !allowed {
			t.Fatalf("Request %d should have been allowed", i+1)
		}
	}

	allowed, _ := mgr.Allow(ctx, clientID, limit, windowSecs)
	if allowed {
		t.Fatal("Should be blocked after limit exhausted")
	}

	time.Sleep(time.Duration(windowSecs+1) * time.Second)

	allowed, _ = mgr.Allow(ctx, clientID, limit, windowSecs)
	if !allowed {
		t.Fatal("Request should be allowed after window expiry")
	}
}

func TestLocalFallback(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:        "localhost:1",
		DialTimeout: 50 * time.Millisecond,
	})
	mgr := limiter.New(rdb)

	for i := 0; i < 5; i++ {
		mgr.HandleRedisFailure()
	}

	if mgr.IsRedisHealthy() {
		t.Fatal("Redis should be marked unhealthy after forced failures")
	}

	ctx := context.Background()
	clientID := "test-local-fallback"
	limit := 5
	windowSecs := 60

	for i := 0; i < limit; i++ {
		allowed, err := mgr.Allow(ctx, clientID, limit, windowSecs)
		if err != nil {
			t.Fatalf("Unexpected error on local fallback request %d: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("Local fallback request %d should have been allowed", i+1)
		}
	}
}

// Race Condition Tests — run with `go test -race`

func TestRaceCondition_ConcurrentRequests(t *testing.T) {
	rdb := newTestRedisClient(t)
	mgr := limiter.New(rdb)
	ctx := context.Background()
	clientID := "test-race-condition"

	rdb.Del(ctx, "limiter:"+clientID)
	defer rdb.Del(ctx, "limiter:"+clientID)

	limit := 50
	windowSecs := 10
	goroutines := 200

	var allowed int64
	var blocked int64
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := mgr.Allow(ctx, clientID, limit, windowSecs)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if ok {
				atomic.AddInt64(&allowed, 1)
			} else {
				atomic.AddInt64(&blocked, 1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Results: allowed=%d, blocked=%d (limit=%d, goroutines=%d)", allowed, blocked, limit, goroutines)

	if allowed > int64(limit)+5 {
		t.Errorf("Race condition detected: too many allowed (%d > %d+tolerance)", allowed, limit)
	}
	if allowed+blocked != int64(goroutines) {
		t.Errorf("Lost requests: allowed(%d) + blocked(%d) != total(%d)", allowed, blocked, goroutines)
	}
}

func TestRaceCondition_MultipleClients(t *testing.T) {
	rdb := newTestRedisClient(t)
	mgr := limiter.New(rdb)
	ctx := context.Background()

	clients := []string{"race-client-a", "race-client-b", "race-client-c"}
	limit := 20
	windowSecs := 10
	requestsPerClient := 30

	for _, c := range clients {
		rdb.Del(ctx, "limiter:"+c)
		defer rdb.Del(ctx, "limiter:"+c)
	}

	results := make(map[string]*int64)
	for _, c := range clients {
		count := new(int64)
		results[c] = count
	}

	var wg sync.WaitGroup
	for _, clientID := range clients {
		for i := 0; i < requestsPerClient; i++ {
			wg.Add(1)
			go func(cid string) {
				defer wg.Done()
				ok, err := mgr.Allow(ctx, cid, limit, windowSecs)
				if err != nil {
					t.Errorf("Error for client %s: %v", cid, err)
					return
				}
				if ok {
					atomic.AddInt64(results[cid], 1)
				}
			}(clientID)
		}
	}

	wg.Wait()

	for cid, countPtr := range results {
		allowed := atomic.LoadInt64(countPtr)
		t.Logf("Client %s: allowed=%d (limit=%d, sent=%d)", cid, allowed, limit, requestsPerClient)
		if allowed > int64(limit)+3 {
			t.Errorf("Client %s: race condition — allowed %d > limit %d", cid, allowed, limit)
		}
	}
}

// Helpers

func newTestRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{
		Addr:        getEnv("REDIS_ADDR", "localhost:6379"),
		DialTimeout: 2 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping: Redis unavailable at %s: %v", rdb.Options().Addr, err)
	}
	return rdb
}
