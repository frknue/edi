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

	"liferpg/internal/agent"
	"liferpg/internal/db"
	"liferpg/internal/handlers"
	"liferpg/internal/services"
)

// singleUserID is the fixed user in MVP single-user mode.
const singleUserID = 1

func main() {
	addr := envOr("LIFERPG_ADDR", ":8080")
	dbPath := envOr("LIFERPG_DB", "liferpg.db")
	clientDir := envOr("LIFERPG_CLIENT_DIR", "../client/dist")

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
	router := handlers.NewRouter(handlers.New(svc, registry), clientDir)

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
