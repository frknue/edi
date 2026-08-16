// Command server is the Life RPG backend: a single self-hosted Go binary that
// runs migrations, seeds demo data on first boot, and serves the REST API (and
// optionally the built web client).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"edi/internal/agent"
	"edi/internal/db"
	"edi/internal/handlers"
	"edi/internal/services"
)

// devUserID is who anonymous requests act as in tokenless dev mode.
const devUserID = 1

func main() {
	addr := listenAddr()
	dbURL := databaseURL()
	clientDir := envOr("EDI_CLIENT_DIR", "../client/dist")
	apiToken := os.Getenv("EDI_TOKEN") // empty = tokenless localhost dev mode

	store, err := db.Open(dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	svc := services.New(store, devUserID)
	if apiToken != "" {
		// Token mode: guarantee user 1 exists (blank admin on an empty DB) and
		// idempotently bind EDI_TOKEN as their login token. Further users come
		// from registration (EDI_INVITE_CODE) or the admin API.
		if err := svc.AdoptEnvToken(apiToken); err != nil {
			log.Fatalf("adopt EDI_TOKEN: %v", err)
		}
		log.Println("per-user token auth enabled (EDI_TOKEN adopted as user 1's token)")
		if services.RegistrationOpen() {
			log.Println("self-serve registration open (EDI_INVITE_CODE set)")
		}
	} else {
		// Dev mode: no auth, anonymous requests act as user 1; seed demo data
		// on a fresh database.
		if err := store.Seed(); err != nil {
			log.Fatalf("seed: %v", err)
		}
	}

	registry := agent.NewRegistry()
	router := handlers.NewRouter(handlers.New(svc, registry), clientDir, apiToken != "")

	srv := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Life RPG server listening on %s (db=%s)", addr, redactDSN(dbURL))
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

// databaseURL resolves the PostgreSQL DSN: EDI_DATABASE_URL wins, then
// DATABASE_URL (injected by Railway when the Postgres service is referenced),
// then a localhost dev default.
func databaseURL() string {
	if v := os.Getenv("EDI_DATABASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://localhost:5432/edi_dev?sslmode=disable"
}

// redactDSN hides the password when logging the connection target.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "postgres"
	}
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.Redacted()
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
