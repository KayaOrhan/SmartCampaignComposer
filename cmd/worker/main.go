package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/config"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/store"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) (result error) {
	cfg, err := config.LoadWorker(os.Getenv)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if db.Close() != nil {
			result = errors.Join(result, errors.New("close worker database failed"))
		}
	}()
	// Dispatch holds one dedicated connection while pacing/doing mock HTTP.
	// Leave spare connections for acknowledgements and lease recovery.
	db.SetMaxOpenConns(2*cfg.Concurrency + 4)
	// The API owns migrations. Fail closed if its worker migration is absent.
	ready, cancel := context.WithTimeout(ctx, cfg.DBTimeout)
	var migrated bool
	err = db.QueryRowContext(ready, `SELECT EXISTS(SELECT 1 FROM public.schema_migrations WHERE version='008_delivery_worker.sql')`).Scan(&migrated)
	cancel()
	if err != nil || !migrated {
		return errors.New("worker schema is not ready")
	}
	client, err := delivery.NewClient(cfg.ProviderURL, cfg.ProviderTimeout)
	if err != nil {
		return err
	}
	defer client.Close()
	worker := delivery.Worker{Queue: store.Deliveries{DB: db, Timeout: cfg.DBTimeout}, Lease: cfg.Lease, Poll: cfg.Poll, Concurrency: cfg.Concurrency, OnError: func(error) { logger.Warn("worker operation failed", "code", "queue_unavailable") }, Send: func(ctx context.Context, j delivery.Job) delivery.Outcome {
		outcome := client.Send(ctx, j)
		logger.Info("mock delivery attempt", "campaign_id", j.CampaignID, "delivery_id", j.ID, "attempt", j.Attempts, "outcome", outcome.Status, "code", outcome.Code)
		return outcome
	}}
	logger.Info("worker starting", "concurrency_per_channel", cfg.Concurrency)
	return worker.Run(ctx)
}
