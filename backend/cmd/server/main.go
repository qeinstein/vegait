package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"rate-limiter/internal/config"
	"rate-limiter/internal/database"
	"rate-limiter/internal/handlers"
	"rate-limiter/internal/limiter"
	"rate-limiter/internal/logger"
	"rate-limiter/internal/middleware"
	"rate-limiter/internal/proxy"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	log.Println("=== Rate Limiter Service Starting ===")

	// Load configuration
	cfg := config.Load()
	log.Printf("Configuration loaded: %s", cfg)

	// Initialize database connections
	dbc, err := database.Init(cfg)
	if err != nil {
		log.Fatalf("FATAL: Failed to initialize databases: %v", err)
	}
	defer dbc.Close()

	// Initialize rate limiter
	lim := limiter.New(dbc.Redis)
	log.Println("Rate limiter initialized")

	// Initialize async logger
	asyncLogger := logger.New(dbc, cfg)
	defer asyncLogger.Close()

	// Build handlers
	proxyHandler := proxy.NewHandler(dbc, lim, asyncLogger, cfg)
	adminHandler := handlers.NewAdmin(dbc, lim, asyncLogger, cfg)

	// --- Gateway Server (proxy) ---
	gatewayMux := http.NewServeMux()
	gatewayMux.Handle("/proxy/", http.StripPrefix("/proxy", proxyHandler))
	gatewayMux.Handle("/proxy", proxyHandler)

	gatewayServer := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      middleware.Chain(gatewayMux, middleware.Recovery, middleware.AccessLog, middleware.RequestID, middleware.ClientID),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// --- Admin Server (dashboard API + health) ---
	adminMux := http.NewServeMux()
	adminHandler.RegisterRoutes(adminMux)

	// Serve dashboard static files if the build directory exists
	dashboardDir := "./dashboard/dist"
	if info, statErr := os.Stat(dashboardDir); statErr == nil && info.IsDir() {
		adminMux.Handle("/", http.FileServer(http.Dir(dashboardDir)))
		log.Printf("Serving dashboard from %s", dashboardDir)
	} else {
		adminMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"service":"rate-limiter-admin","dashboard":"not built"}`))
		})
	}

	adminServer := &http.Server{
		Addr:         ":" + cfg.AdminPort,
		Handler:      middleware.Chain(adminMux, middleware.Recovery, middleware.AccessLog, middleware.CORS, middleware.RequestID),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	// --- Start servers ---
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		log.Printf("Gateway server listening on :%s", cfg.ServerPort)
		if err := gatewayServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("FATAL: Gateway server error: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		log.Printf("Admin server listening on :%s", cfg.AdminPort)
		if err := adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("FATAL: Admin server error: %v", err)
		}
	}()

	// --- Graceful Shutdown ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received signal %s. Initiating graceful shutdown...", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	gatewayServer.Shutdown(shutdownCtx)
	adminServer.Shutdown(shutdownCtx)
	asyncLogger.Close()

	log.Println("=== Rate Limiter Service Stopped ===")
}
