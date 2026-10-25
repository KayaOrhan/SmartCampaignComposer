package httpapi

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (a campaignAPI) delete(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	expected, err := requiredRevision(r, id)
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
	if err := a.store.Delete(r.Context(), id, expected, a.now().UTC()); err != nil {
		fail(w, err)
		return
	}
	slog.Info("campaign deleted", "campaign_id", id)
	w.WriteHeader(http.StatusNoContent)
}
