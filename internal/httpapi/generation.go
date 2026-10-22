package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type SegmentGenerator interface {
	ExtractSegment(context.Context, int64, string) (segment.Validated, error)
}

func registerGeneration(mux *http.ServeMux, store CampaignStore, generator SegmentGenerator, timeout time.Duration) {
	mux.HandleFunc("POST /campaigns/{id}/generate-segment", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		id, err := campaignID(r)
		if err != nil {
			fail(w, err)
			return
		}
		body, err := readBody(w, r)
		if err != nil {
			fail(w, err)
			return
		}
		if strings.TrimSpace(string(body)) != "" {
			fail(w, segment.Invalid("body_not_allowed", "$"))
			return
		}
		expected, err := requiredRevision(r, id)
		if err != nil {
			fail(w, err)
			return
		}
		c, err := store.Get(ctx, id)
		if err != nil {
			fail(w, err)
			return
		}
		if c.Revision != expected {
			fail(w, campaign.ErrRevision)
			return
		}
		if !campaign.Editable(c.Status) {
			fail(w, campaign.ErrState)
			return
		}
		v, err := generator.ExtractSegment(ctx, id, c.Brief)
		if err != nil {
			var e *llm.Error
			if errors.As(err, &e) {
				status := http.StatusBadGateway
				if e.Code == "llm_timeout" {
					status = http.StatusGatewayTimeout
				}
				if e.Code == "llm_unavailable" || e.Code == "llm_audit_unavailable" {
					status = http.StatusServiceUnavailable
				}
				writeJSON(w, status, map[string]any{"error": e})
			} else {
				fail(w, err)
			}
			return
		}
		// PutSegment locks and rechecks revision/editable state after generation. No DB
		// transaction or row lock is held while the model is running.
		c, err = store.PutSegment(ctx, id, expected, v)
		if err != nil {
			fail(w, err)
			return
		}
		slog.Info("campaign segment generated", "campaign_id", id, "revision", c.Revision)
		w.Header().Set("ETag", revisionTag(c.ID, c.Revision))
		writeJSON(w, http.StatusOK, c)
	})
}
