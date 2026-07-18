package tests

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)


func TestLoad_Throughput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	gatewayAddr := getEnv("GATEWAY_ADDR", "http://localhost:8080")
	targetPath := "/proxy/load-test"
	clientID := "load-test-client"
	concurrency := 50
	duration := 10 * time.Second

	t.Logf("Load test: %d concurrent workers for %v against %s", concurrency, duration, gatewayAddr)

	var totalRequests, totalAllowed, totalBlocked, totalErrors, totalLatencyNs int64
	var wg sync.WaitGroup
	stop := make(chan struct{})

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        concurrency * 2,
			MaxIdleConnsPerHost: concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}

				req, _ := http.NewRequest(http.MethodGet, gatewayAddr+targetPath, nil)
				req.Header.Set("X-Client-ID", clientID)

				start := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(start)

				atomic.AddInt64(&totalRequests, 1)
				atomic.AddInt64(&totalLatencyNs, elapsed.Nanoseconds())

				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
					continue
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()

				switch resp.StatusCode {
				case http.StatusOK:
					atomic.AddInt64(&totalAllowed, 1)
				case http.StatusTooManyRequests:
					atomic.AddInt64(&totalBlocked, 1)
				default:
					atomic.AddInt64(&totalErrors, 1)
				}
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()

	total := atomic.LoadInt64(&totalRequests)
	avgLatencyMs := float64(0)
	if total > 0 {
		avgLatencyMs = float64(atomic.LoadInt64(&totalLatencyNs)) / float64(total) / 1e6
	}
	rps := float64(total) / duration.Seconds()

	t.Logf("==========================================")
	t.Logf("  Load Test Results")
	t.Logf("==========================================")
	t.Logf("  Duration:       %v", duration)
	t.Logf("  Concurrency:    %d", concurrency)
	t.Logf("  Total Requests: %d", total)
	t.Logf("  Allowed:        %d", atomic.LoadInt64(&totalAllowed))
	t.Logf("  Blocked (429):  %d", atomic.LoadInt64(&totalBlocked))
	t.Logf("  Errors:         %d", atomic.LoadInt64(&totalErrors))
	t.Logf("  Throughput:     %.0f req/sec", rps)
	t.Logf("  Avg Latency:    %.2f ms", avgLatencyMs)
	t.Logf("==========================================")

	if avgLatencyMs > 50 {
		t.Errorf("Average latency too high: %.2fms > 50ms", avgLatencyMs)
	}
}

func TestLoad_BurstRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst recovery test in short mode")
	}

	gatewayAddr := getEnv("GATEWAY_ADDR", "http://localhost:8080")
	clientID := "client-alpha"
	client := &http.Client{Timeout: 5 * time.Second}

	// Settle before bursting. If a prior test (e.g. TestLoad_Throughput) just
	// hammered the shared gateway, Redis may still be draining — and the gateway's
	// aggressive 5ms limiter timeout would fail *open* under that pressure, letting
	// the burst through and making this assertion flaky. A brief pause lets the
	// gateway recover and client-alpha's 10s window roll over to a clean slate.
	time.Sleep(11 * time.Second)

	var allowed, blocked int
	for i := 0; i < 15; i++ {
		req, _ := http.NewRequest(http.MethodGet, gatewayAddr+"/proxy/burst-test", nil)
		req.Header.Set("X-Client-ID", clientID)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request %d failed: %v", i+1, err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			allowed++
		} else if resp.StatusCode == http.StatusTooManyRequests {
			blocked++
		}
	}

	t.Logf("Phase 1 (burst): allowed=%d, blocked=%d", allowed, blocked)
	if blocked == 0 {
		t.Error("Expected some requests to be blocked during burst")
	}

	t.Log("Waiting 11 seconds for window to expire...")
	time.Sleep(11 * time.Second)

	req, _ := http.NewRequest(http.MethodGet, gatewayAddr+"/proxy/recovery-test", nil)
	req.Header.Set("X-Client-ID", clientID)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Recovery request failed: %v", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 after window expiry, got %d", resp.StatusCode)
	}
	t.Log("Phase 2 (recovery): request allowed ✓")
}

func BenchmarkRateLimitCheck(b *testing.B) {
	gatewayAddr := getEnv("GATEWAY_ADDR", "http://localhost:8080")
	client := &http.Client{Timeout: 5 * time.Second}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest(http.MethodGet,
			fmt.Sprintf("%s/proxy/bench-%d", gatewayAddr, i), nil)
		req.Header.Set("X-Client-ID", fmt.Sprintf("bench-client-%d", i%100))
		resp, err := client.Do(req)
		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}
}
