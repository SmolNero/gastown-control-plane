package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SmolNero/gastown-control-plane/internal/config"
	"github.com/SmolNero/gastown-control-plane/internal/db"
	"github.com/SmolNero/gastown-control-plane/internal/server"
	"github.com/SmolNero/gastown-control-plane/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.FromEnv()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connection failed: %v", err)
	}
	defer pool.Close()

	if cfg.AutoMigrate {
		if err := db.Migrate(ctx, pool); err != nil {
			log.Fatalf("migrations failed: %v", err)
		}
	}

	st := store.New(pool)
	api := server.New(st, server.Config{
		MaxEventBytes:        cfg.MaxEventBytes,
		MaxSnapshotBytes:     cfg.MaxSnapshotBytes,
		RateLimitPerMinute:   cfg.RateLimitPerMinute,
		SchemaVersion:        1,
		Version:              cfg.Version,
		AgentDownloadBaseURL: cfg.AgentDownloadBaseURL,
	})

	if cfg.EventRetentionDays > 0 || cfg.SnapshotRetentionDays > 0 {
		go runPruneLoop(ctx, st, cfg.EventRetentionDays, cfg.SnapshotRetentionDays, cfg.PruneInterval)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("control plane listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func runPruneLoop(ctx context.Context, st *store.Store, eventDays, snapshotDays int, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if eventDays > 0 {
				before := now.Add(-time.Duration(eventDays) * 24 * time.Hour)
				_, _ = st.PruneEvents(ctx, before)
			}
			if snapshotDays > 0 {
				before := now.Add(-time.Duration(snapshotDays) * 24 * time.Hour)
				_, _ = st.PruneSnapshots(ctx, before)
			}
		}
	}
}
