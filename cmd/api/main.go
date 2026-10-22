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
	"github.com/KayaOrhan/SmartCampaignComposer/internal/httpapi"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/store"
	"github.com/KayaOrhan/SmartCampaignComposer/seed"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
			if len(groups) == 0 && attr.Key == slog.TimeKey {
				return slog.Time(slog.TimeKey, attr.Value.Time().UTC())
			}
			return attr
		},
	}))
	slog.SetDefault(logger)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) (result error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	mock, err := llm.NewMock(cfg.LLMMockScenario)
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.DBTimeout)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			result = errors.Join(result, errors.New("close database pool failed"))
		}
	}()
	if err := store.Migrate(ctx, db, 30*time.Second); err != nil {
		return err
	}
	if cfg.SeedData {
		if err := seed.Ensure(ctx, db, time.Now().UTC()); err != nil {
			return err
		}
	}
	repo := store.Campaigns{DB: db, Timeout: cfg.DBTimeout}
	srv := &http.Server{Addr: cfg.Address, Handler: httpapi.Handler(db.PingContext, cfg.DBTimeout, repo, llm.SegmentGenerator{Provider: mock, Recorder: repo, Timeout: cfg.LLMTimeout}, cfg.LLMTimeout+2*cfg.DBTimeout), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: cfg.LLMTimeout + 2*cfg.DBTimeout + 5*time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan error, 1)
	go func() { stopped <- srv.ListenAndServe() }()
	logger.Info("api starting", "address", cfg.Address)
	select {
	case err := <-stopped:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server failed")
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			closeErr := srv.Close()
			if closeErr != nil {
				return errors.New("HTTP shutdown and close failed")
			}
			return errors.New("HTTP shutdown deadline exceeded")
		}
		if err := <-stopped; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server failed during shutdown")
		}
		return nil
	}
}
