// Package httputil provides shared HTTP response helpers used across
// the handlers and proxy packages.
package httputil

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// WriteJSON serializes payload as JSON and writes it to the response.
func WriteJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// QueryInt reads an integer query parameter with a fallback default.
func QueryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
