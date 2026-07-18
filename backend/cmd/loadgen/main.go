// Command loadgen is a self-contained, dependency-free load generator for the
// Rate Limiter gateway. It fires a large volume of concurrent requests
// (>10k by default) and prints a minimalist, New Relic-style terminal report
// with throughput and latency percentiles.
//
// Usage:
//
//	go run ./cmd/loadgen -n 12000 -c 150 -client client-gamma
//
// All flags have sane defaults, so `go run ./cmd/loadgen` "just works" against
// a gateway on localhost:8080.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ANSI colors — a restrained, New Relic-inspired palette on a dark terminal.
// These are vars (not consts) so NO_COLOR can blank them all at startup.
var (
	reset = "\033[0m"
	bold  = "\033[1m"
	dim   = "\033[2m"
	green = "\033[38;5;42m"  // New Relic signature green
	teal  = "\033[38;5;44m"  // accent cyan/teal
	amber = "\033[38;5;214m" // warnings / blocked
	red   = "\033[38;5;203m" // errors
	slate = "\033[38;5;245m" // muted labels
	white = "\033[38;5;255m"
)

type config struct {
	base        string
	path        string
	clientID    string
	total       int
	concurrency int
	timeout     time.Duration
}

// result captures the outcome of a single request.
type result struct {
	status  int
	latency time.Duration
	err     bool
}

func main() {
	cfg := parseFlags()

	if noColor() {
		disableColors()
	}

	printBanner(cfg)

	// Shared counters (atomic for the live ticker).
	var sent, allowed, blocked, errors int64
	var otherStatus int64

	// Per-worker latency slices are merged at the end to avoid lock contention
	// on the hot path.
	perWorker := make([][]time.Duration, cfg.concurrency)

	client := &http.Client{
		Timeout: cfg.timeout,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.concurrency * 2,
			MaxIdleConnsPerHost: cfg.concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	// jobs distributes the fixed number of requests across workers.
	jobs := make(chan struct{}, cfg.concurrency)
	var wg sync.WaitGroup

	start := time.Now()

	// Live progress ticker.
	doneCh := make(chan struct{})
	go liveTicker(cfg, start, &sent, &allowed, &blocked, &errors, doneCh)

	for w := 0; w < cfg.concurrency; w++ {
		wg.Add(1)
		lat := make([]time.Duration, 0, cfg.total/cfg.concurrency+1)
		go func(workerID int, lat []time.Duration) {
			defer wg.Done()
			for range jobs {
				r := fire(client, cfg)
				atomic.AddInt64(&sent, 1)
				switch {
				case r.err:
					atomic.AddInt64(&errors, 1)
				case r.status == http.StatusOK:
					atomic.AddInt64(&allowed, 1)
					lat = append(lat, r.latency)
				case r.status == http.StatusTooManyRequests:
					atomic.AddInt64(&blocked, 1)
				default:
					atomic.AddInt64(&otherStatus, 1)
				}
			}
			perWorker[workerID] = lat
		}(w, lat)
	}

	for i := 0; i < cfg.total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	close(doneCh)

	elapsed := time.Since(start)

	// Merge latencies.
	latencies := make([]time.Duration, 0, cfg.total)
	for _, s := range perWorker {
		latencies = append(latencies, s...)
	}

	printReport(cfg, elapsed, sent, allowed, blocked, errors, otherStatus, latencies)
}

func parseFlags() config {
	var cfg config
	flag.StringVar(&cfg.base, "url", envStr("GATEWAY_ADDR", "http://localhost:8080"), "gateway base URL")
	flag.StringVar(&cfg.path, "path", "/proxy/loadtest", "proxy path to hit")
	flag.StringVar(&cfg.clientID, "client", "client-gamma", "X-Client-ID to send")
	flag.IntVar(&cfg.total, "n", envInt("LOADGEN_TOTAL", 12000), "total number of requests")
	flag.IntVar(&cfg.concurrency, "c", envInt("LOADGEN_CONCURRENCY", 150), "concurrent workers")
	timeoutMs := flag.Int("timeout-ms", 10000, "per-request timeout in milliseconds")
	flag.Parse()

	if cfg.concurrency < 1 {
		cfg.concurrency = 1
	}
	if cfg.total < 1 {
		cfg.total = 1
	}
	cfg.timeout = time.Duration(*timeoutMs) * time.Millisecond
	return cfg
}

func fire(client *http.Client, cfg config) result {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.base+cfg.path, nil)
	if err != nil {
		return result{err: true}
	}
	req.Header.Set("X-Client-ID", cfg.clientID)

	begin := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return result{err: true}
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return result{status: resp.StatusCode, latency: time.Since(begin)}
}

// liveTicker prints an in-place progress line a few times per second.
func liveTicker(cfg config, start time.Time, sent, allowed, blocked, errors *int64, done <-chan struct{}) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s := atomic.LoadInt64(sent)
			el := time.Since(start).Seconds()
			rps := float64(0)
			if el > 0 {
				rps = float64(s) / el
			}
			pct := float64(s) / float64(cfg.total) * 100
			bar := progressBar(pct, 28)
			fmt.Fprintf(os.Stderr,
				"\r%s %s%5.1f%%%s  %ssent%s %s%-7d%s %sok%s %s%-7d%s %s429%s %s%-7d%s %serr%s %s%-5d%s %s%.0f req/s%s   ",
				bar, white, pct, reset,
				slate, reset, green, s, reset,
				slate, reset, green, atomic.LoadInt64(allowed), reset,
				slate, reset, amber, atomic.LoadInt64(blocked), reset,
				slate, reset, red, atomic.LoadInt64(errors), reset,
				teal, rps, reset,
			)
		}
	}
}

func progressBar(pct float64, width int) string {
	if pct > 100 {
		pct = 100
	}
	filled := int(pct / 100 * float64(width))
	out := teal + "["
	for i := 0; i < width; i++ {
		if i < filled {
			out += "="
		} else if i == filled {
			out += ">"
		} else {
			out += " "
		}
	}
	out += "]" + reset
	return out
}

func printBanner(cfg config) {
	fmt.Printf("\n%s%s  RATE LIMITER · LOAD GENERATOR%s\n", bold, green, reset)
	fmt.Printf("%s  ────────────────────────────────────────────%s\n", dim, reset)
	fmt.Printf("  %starget%s      %s%s%s\n", slate, reset, white, cfg.base+cfg.path, reset)
	fmt.Printf("  %sclient%s     %s%s%s\n", slate, reset, white, cfg.clientID, reset)
	fmt.Printf("  %srequests%s   %s%d%s   %sconcurrency%s %s%d%s\n\n",
		slate, reset, white, cfg.total, reset, slate, reset, white, cfg.concurrency, reset)
}

func printReport(cfg config, elapsed time.Duration, sent, allowed, blocked, errors, other int64, latencies []time.Duration) {
	fmt.Fprint(os.Stderr, "\r"+padTo("", 110)+"\r") // clear the live line

	rps := float64(sent) / elapsed.Seconds()
	successRate := float64(allowed) / math.Max(float64(sent), 1) * 100

	fmt.Printf("\n%s%s  RESULTS%s\n", bold, green, reset)
	fmt.Printf("%s  ══════════════════════════════════════════════%s\n", dim, reset)

	row := func(label, val, color string) {
		fmt.Printf("  %s%-18s%s %s%s%s\n", slate, label, reset, color, val, reset)
	}

	row("Duration", fmt.Sprintf("%.2fs", elapsed.Seconds()), white)
	row("Total sent", fmt.Sprintf("%d", sent), white)
	row("Throughput", fmt.Sprintf("%.0f req/s", rps), teal)
	fmt.Println()
	row("Allowed (200)", fmt.Sprintf("%d  (%.1f%%)", allowed, successRate), green)
	row("Rate-limited (429)", fmt.Sprintf("%d", blocked), amber)
	if other > 0 {
		row("Other status", fmt.Sprintf("%d", other), amber)
	}
	row("Errors", fmt.Sprintf("%d", errors), pickColor(errors == 0, green, red))

	if len(latencies) > 0 {
		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		fmt.Printf("\n%s  LATENCY%s %s(allowed requests, end-to-end through gateway)%s\n", bold+white, reset, dim, reset)
		fmt.Printf("%s  ──────────────────────────────────────────────%s\n", dim, reset)
		latRow := func(label string, d time.Duration, color string) {
			fmt.Printf("  %s%-10s%s %s%8.2f ms%s  %s%s%s\n",
				slate, label, reset, color, msf(d), reset, dim, latBar(d, latencies), reset)
		}
		latRow("min", latencies[0], green)
		latRow("p50", pct(latencies, 50), green)
		latRow("p90", pct(latencies, 90), teal)
		latRow("p95", pct(latencies, 95), teal)
		latRow("p99", pct(latencies, 99), amber)
		latRow("max", latencies[len(latencies)-1], amber)
		latRow("avg", avg(latencies), white)
	}

	fmt.Println()
	verdict := allowed > 0 && errors == 0
	if verdict {
		fmt.Printf("  %s%s✔ Gateway healthy — %d requests proxied, rate limiting enforced.%s\n\n",
			bold, green, allowed, reset)
	} else if allowed > 0 {
		fmt.Printf("  %s%s⚠ Completed with %d transport errors.%s\n\n", bold, amber, errors, reset)
	} else {
		fmt.Printf("  %s%s✘ No successful requests — is the stack running? (run.sh)%s\n\n", bold, red, reset)
	}
}

// --- small helpers ---

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func avg(ds []time.Duration) time.Duration {
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	return sum / time.Duration(len(ds))
}

func msf(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

// latBar draws a tiny sparkline-style bar scaled against the p99 of the set.
func latBar(d time.Duration, sorted []time.Duration) string {
	max := pct(sorted, 99)
	if max <= 0 {
		return ""
	}
	width := 24
	n := int(float64(d) / float64(max) * float64(width))
	if n > width {
		n = width
	}
	out := ""
	for i := 0; i < n; i++ {
		out += "▏"
	}
	return out
}

func pickColor(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func padTo(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

func noColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

func disableColors() {
	reset, bold, dim = "", "", ""
	green, teal, amber, red, slate, white = "", "", "", "", "", ""
}

func envStr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, ok := os.LookupEnv(k); ok {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
