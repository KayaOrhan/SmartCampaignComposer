package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type CampaignStore interface {
	Create(context.Context, campaign.Draft) (campaign.Campaign, error)
	Get(context.Context, int64) (campaign.Campaign, error)
	PutSegment(context.Context, int64, segment.Validated) (campaign.Campaign, error)
	Preview(context.Context, int64, time.Time) (campaign.Preview, error)
}

type campaignAPI struct {
	store CampaignStore
	now   func() time.Time
}

func registerCampaigns(mux *http.ServeMux, store CampaignStore, now func() time.Time, timeout time.Duration) {
	a := campaignAPI{store: store, now: now}
	bounded := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next(w, r.WithContext(ctx))
		}
	}
	mux.HandleFunc("POST /campaigns", bounded(a.create))
	mux.HandleFunc("GET /campaigns/{id}", bounded(a.get))
	mux.HandleFunc("PUT /campaigns/{id}/segment", bounded(a.putSegment))
	mux.HandleFunc("POST /campaigns/{id}/audience-preview", bounded(a.preview))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	encoded, err := json.Marshal(value)
	if err != nil {
		respond(w, 500, `{"error":{"code":"internal_error","path":"$"}}`)
		return
	}
	respond(w, status, string(encoded))
}

func fail(w http.ResponseWriter, err error) {
	code, path, status := "database_unavailable", "$", 503
	var v *segment.ValidationError
	switch {
	case errors.As(err, &v):
		code, path, status = v.Code, v.Path, 422
		if code == "invalid_json" {
			status = 400
		}
		if code == "body_too_large" {
			status = 413
		}
	case errors.Is(err, campaign.ErrNotFound):
		code, status = "campaign_not_found", 404
	case errors.Is(err, campaign.ErrState):
		code, status = "invalid_campaign_state", 409
	case errors.Is(err, campaign.ErrSegmentRequired):
		code, status = "segment_required", 409
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "path": path}})
}

func readJSONBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return nil, segment.Invalid("json_content_type_required", "$")
	}
	return readBody(w, r)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, segment.MaxJSONBytes))
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			return nil, segment.Invalid("body_too_large", "$")
		}
		return nil, segment.Invalid("invalid_json", "$")
	}
	return body, nil
}

func campaignID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, segment.Invalid("invalid_campaign_id", "id")
	}
	return id, nil
}

func (a campaignAPI) create(w http.ResponseWriter, r *http.Request) {
	body, err := readJSONBody(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	var draft campaign.Draft
	if err := segment.DecodeStrict(body, &draft); err != nil {
		fail(w, err)
		return
	}
	draft, err = draft.Validate()
	if err != nil {
		fail(w, err)
		return
	}
	c, err := a.store.Create(r.Context(), draft)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Location", "/campaigns/"+strconv.FormatInt(c.ID, 10))
	slog.Info("campaign created", "campaign_id", c.ID)
	writeJSON(w, 201, c)
}

func (a campaignAPI) get(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	c, err := a.store.Get(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, c)
}

func (a campaignAPI) putSegment(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	body, err := readJSONBody(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	v, err := segment.Parse(body)
	if err != nil {
		fail(w, err)
		return
	}
	c, err := a.store.PutSegment(r.Context(), id, v)
	if err != nil {
		fail(w, err)
		return
	}
	slog.Info("campaign segment saved", "campaign_id", id)
	writeJSON(w, 200, c)
}

func (a campaignAPI) preview(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	// Preview inputs come only from the validated stored campaign.
	body, err := readBody(w, r)
	if err != nil {
		fail(w, err)
		return
	}
	if strings.TrimSpace(string(body)) != "" {
		fail(w, segment.Invalid("body_not_allowed", "$"))
		return
	}
	p, err := a.store.Preview(r.Context(), id, a.now().UTC())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}
