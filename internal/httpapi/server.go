package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Handler takes a context-aware database check so unit tests need no database.
func Handler(ping func(context.Context) error, timeout time.Duration, campaigns CampaignStore, generator SegmentGenerator, generationTimeout time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := ping(ctx); err != nil {
			respond(w, http.StatusServiceUnavailable, `{"status":"unavailable"}`)
			return
		}
		respond(w, http.StatusOK, `{"status":"ready"}`)
	})
	if campaigns != nil {
		registerCampaigns(mux, campaigns, time.Now, timeout)
		if generator != nil {
			registerGeneration(mux, campaigns, generator, generationTimeout)
		}
	}
	return mux
}

func respond(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	// Never include client data or the raw write error in logs.
	if _, err := io.WriteString(w, body+"\n"); err != nil {
		slog.Warn("health response write failed")
		return
	}
}
