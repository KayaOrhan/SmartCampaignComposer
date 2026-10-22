package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (a campaignAPI) approve(w http.ResponseWriter, r *http.Request) {
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
		Fingerprint string `json:"approval_fingerprint"`
	}
	if err := segment.DecodeStrict(body, &input); err != nil {
		fail(w, err)
		return
	}
	if err := campaign.ValidateApprovalFingerprint(input.Fingerprint); err != nil {
		fail(w, err)
		return
	}
	expected, err := requiredRevision(r, id)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := a.store.Approve(r.Context(), id, expected, input.Fingerprint, a.now().UTC())
	if err != nil {
		var changed *campaign.ApprovalPreviewChanged
		if errors.As(err, &changed) {
			writeJSON(w, 409, map[string]any{"error": map[string]string{"code": "approval_preview_changed", "path": "approval_fingerprint"}, "preview": changed.Preview})
		} else {
			fail(w, err)
		}
		return
	}
	slog.Info("campaign approval recorded", "campaign_id", id, "revision", result.Revision)
	w.Header().Set("ETag", revisionTag(result.CampaignID, result.Revision))
	writeJSON(w, 200, result)
}

func (a campaignAPI) deliveryStatus(w http.ResponseWriter, r *http.Request) {
	id, err := campaignID(r)
	if err != nil {
		fail(w, err)
		return
	}
	result, err := a.store.GetDeliveryStatus(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, result)
}
