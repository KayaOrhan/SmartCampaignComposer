package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type MessageGenerator interface {
	GenerateMessages(context.Context, int64, string, []string, campaign.Preview, llm.Emit) (campaign.ValidatedMessages, error)
}

func registerMessageGeneration(mux *http.ServeMux, store CampaignStore, generator MessageGenerator, now func() time.Time, timeout time.Duration) {
	mux.HandleFunc("POST /campaigns/{id}/generate-messages", func(w http.ResponseWriter, r *http.Request) {
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
		c, err := store.Get(ctx, id)
		if err != nil {
			fail(w, err)
			return
		}
		if c.Status != "draft" {
			fail(w, campaign.ErrState)
			return
		}
		if len(c.Segment) == 0 {
			fail(w, campaign.ErrSegmentRequired)
			return
		}
		audience, err := store.Preview(ctx, id, now().UTC())
		if err != nil {
			fail(w, err)
			return
		}
		if _, ok := w.(http.Flusher); !ok {
			fail(w, errors.New("streaming unsupported"))
			return
		}
		controller := http.NewResponseController(w)
		deadline, _ := ctx.Deadline()
		if err := controller.SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		writeEvent := func(kind string, value any) error {
			encoded, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded); err != nil {
				return err
			}
			return controller.Flush()
		}
		attempt := 0
		v, err := generator.GenerateMessages(ctx, id, c.Brief, c.Channels, audience, func(kind string, event llm.Event) error {
			if kind != "attempt" && kind != "delta" {
				return errors.New("unexpected stream event")
			}
			attempt = event.Attempt
			err := writeEvent(kind, event)
			if err != nil {
				cancel()
			}
			return err
		})
		if err == nil && len(v.JSON()) == 0 {
			err = &llm.Error{Code: "llm_output_invalid", Path: "$"}
		}
		if ctx.Err() != nil && err == nil {
			err = &llm.Error{Code: "llm_timeout", Path: "$"}
		}
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			e := &llm.Error{Code: "llm_unavailable", Path: "$"}
			var typed *llm.Error
			if errors.As(err, &typed) {
				e = typed
			}
			if writeErr := writeEvent("error", map[string]any{"attempt": attempt, "error": e}); writeErr != nil {
				return
			}
			return
		}
		// Preview only: do not save templates or advance campaign state.
		if err := writeEvent("validated", map[string]any{"attempt": attempt, "messages": v.JSON(), "audience": audience}); err != nil {
			return
		}
	})
}
