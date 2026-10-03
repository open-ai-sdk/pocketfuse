package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/open-ai-sdk/pocketfuse/internal/server"
	"github.com/open-ai-sdk/pocketfuse/internal/store"
)

func main() {
	dbPath := env("POCKETFUSE_DB_PATH", "./pocketfuse.db")
	if dbPath != ":memory:" {
		if parent := filepath.Dir(dbPath); parent != "." && parent != "" {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				log.Fatalf("create database directory: %v", err)
			}
		}
	}
	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()

	address := env("POCKETFUSE_HOST", "127.0.0.1") + ":" + env("POCKETFUSE_PORT", "7625")
	httpServer := &http.Server{Addr: address, Handler: server.New(db).Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 45 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second}
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdown)
	}()
	log.Printf("pocketfuse listening on http://%s (db=%s)", address, dbPath)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
