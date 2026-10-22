package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (a campaignAPI) getMessages(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := a.store.GetMessages(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("ETag", revisionTag(result.CampaignID, result.Revision))
	writeJSON(w, 200, result)
}

func (a campaignAPI) putMessages(w http.ResponseWriter, r *http.Request) {
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
	var input struct {
		Messages json.RawMessage `json:"messages"`
	}
	if err := segment.DecodeStrict(body, &input); err != nil {
		fail(w, err)
		return
	}
	expected, err := requiredRevision(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := a.store.PutMessages(r.Context(), id, expected, input.Messages, a.now().UTC())
	if err != nil {
		fail(w, err)
		return
	}
	slog.Info("campaign messages saved", "campaign_id", id, "revision", result.Revision)
	w.Header().Set("ETag", revisionTag(result.CampaignID, result.Revision))
	writeJSON(w, 200, result)
}
