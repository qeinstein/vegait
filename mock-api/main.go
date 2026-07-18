package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

// mock-api simulates a third-party API with variable latency so we can
// load-test the rate limiter without hitting real external services.

func main() {
	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = "9090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleMock)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	log.Printf("Mock API server listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Mock API server failed: %v", err)
	}
}

func handleMock(w http.ResponseWriter, r *http.Request) {
	// Simulate variable latency between 10ms and 200ms
	latency := time.Duration(10+rand.Intn(190)) * time.Millisecond
	time.Sleep(latency)

	// Simulate occasional errors (5% chance)
	if rand.Float64() < 0.05 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "simulated upstream error",
			"latency": fmt.Sprintf("%dms", latency.Milliseconds()),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "mock response",
		"path":       r.URL.Path,
		"method":     r.Method,
		"latency_ms": latency.Milliseconds(),
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	})
}
