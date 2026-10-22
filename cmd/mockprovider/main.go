package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/config"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/mockprovider"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/store"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		slog.Error("mock provider stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (result error) {
	cfg, err := config.LoadMock(os.Getenv)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if db.Close() != nil {
			result = errors.Join(result, errors.New("close mock database failed"))
		}
	}()
	ready, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
	var migrated bool
	err = db.QueryRowContext(ready, `SELECT EXISTS(SELECT 1 FROM public.schema_migrations WHERE version='008_delivery_worker.sql')`).Scan(&migrated)
	cancel()
	if err != nil || !migrated {
		return errors.New("mock schema is not ready")
	}
	provider := mockprovider.Provider{DB: db, Timeout: cfg.DBTimeout + cfg.Latency, Seed: cfg.Seed, Rate5xx: cfg.Rate5xx, Rate429: cfg.Rate429, Latency: cfg.Latency}
	srv := &http.Server{Addr: cfg.Address, Handler: provider.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- srv.ListenAndServe() }()
	select {
	case err := <-stopped:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("mock HTTP server failed")
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
			return errors.New("mock shutdown failed")
		}
		if err := <-stopped; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.New("mock HTTP shutdown failed")
		}
		return nil
	}
}
