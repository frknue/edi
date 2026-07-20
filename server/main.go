// Command server is the Life RPG backend: a single self-hosted Go binary that
// runs migrations, seeds demo data on first boot, and serves the REST API (and
// optionally the built web client).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"edi/internal/agent"
	"edi/internal/db"
	"edi/internal/handlers"
	"edi/internal/services"
)

// singleUserID is the fixed user in MVP single-user mode.
const singleUserID = 1

func main() {
	addr := listenAddr()
	dbPath := envOr("EDI_DB", "edi.db")
	clientDir := envOr("EDI_CLIENT_DIR", "../client/dist")
	apiToken := os.Getenv("EDI_TOKEN") // empty = no auth (localhost default)

	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	if err := store.Seed(); err != nil {
		log.Fatalf("seed: %v", err)
	}

	svc := services.New(store, singleUserID)
	registry := agent.NewRegistry(svc)
	router := handlers.NewRouter(handlers.New(svc, registry), clientDir, apiToken)
	if apiToken != "" {
		log.Println("API token auth enabled (EDI_TOKEN set)")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Life RPG server listening on %s (db=%s)", addr, dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown failed (%v); forcing close", err)
		_ = srv.Close()
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// listenAddr resolves the listen address: EDI_ADDR wins, then PORT (injected by
// PaaS hosts like Railway), then the :8080 default.
func listenAddr() string {
	if v := os.Getenv("EDI_ADDR"); v != "" {
		return v
	}
	if p := os.Getenv("PORT"); p != "" {
		return ":" + p
	}
	return ":8080"
}
