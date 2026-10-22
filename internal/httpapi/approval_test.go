package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

type approvalHTTPStore struct {
	fakeCampaignStore
	expected    int64
	fingerprint string
}

func (s *approvalHTTPStore) Approve(ctx context.Context, id, expected int64, fingerprint string, at time.Time) (campaign.DeliveryStatus, error) {
	s.called(ctx)
	s.expected = expected
	s.fingerprint = fingerprint
	s.asOf = at
	return campaign.DeliveryStatus{CampaignID: id, Revision: expected + 1, Status: "sending", Counts: campaign.DeliveryCounts{Total: 2, Pending: 2}, ByChannel: map[string]campaign.DeliveryCounts{"sms": {Total: 2, Pending: 2}}}, s.err
}

func TestApprovalHTTPContracts(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	valid := `{"approval_fingerprint":"` + fingerprint + `"}`
	changed := &campaign.ApprovalPreviewChanged{Preview: campaign.ApprovalPreview{Revision: 7, ApprovalFingerprint: &fingerprint, Preview: campaign.Preview{AsOf: time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC), EligibleUnique: 2, ByChannel: map[string]campaign.Bounds{"sms": {Count: 2, MaxFirstName: 3, MaxCity: 8}}}}}
	for _, tc := range []struct {
		name, body, tag string
		err             error
		status, calls   int
	}{
		{"approve", valid, `"campaign-1-r7"`, nil, 200, 1},
		{"missing revision", valid, "", nil, 428, 0},
		{"invalid revision", valid, "*", nil, 400, 0},
		{"missing fingerprint", `{}`, `"campaign-1-r7"`, nil, 422, 0},
		{"null fingerprint", `{"approval_fingerprint":null}`, `"campaign-1-r7"`, nil, 422, 0},
		{"malformed JSON", `{`, `"campaign-1-r7"`, nil, 400, 0},
		{"unknown field", `{"approval_fingerprint":"` + fingerprint + `","status":"approved"}`, `"campaign-1-r7"`, nil, 422, 0},
		{"duplicate field", `{"approval_fingerprint":"` + fingerprint + `","approval_fingerprint":"` + fingerprint + `"}`, `"campaign-1-r7"`, nil, 400, 0},
		{"oversized", strings.Repeat(" ", 32769), `"campaign-1-r7"`, nil, 413, 0},
		{"stale", valid, `"campaign-1-r7"`, campaign.ErrRevision, 412, 1},
		{"state", valid, `"campaign-1-r7"`, campaign.ErrState, 409, 1},
		{"empty", valid, `"campaign-1-r7"`, campaign.ErrEmptyAudience, 409, 1},
		{"changed preview", valid, `"campaign-1-r7"`, changed, 409, 1},
		{"concurrent", valid, `"campaign-1-r7"`, campaign.ErrConcurrentUpdate, 409, 1},
		{"missing", valid, `"campaign-1-r7"`, campaign.ErrNotFound, 404, 1},
		{"database", valid, `"campaign-1-r7"`, errors.New("private database detail"), 503, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &approvalHTTPStore{fakeCampaignStore: fakeCampaignStore{err: tc.err}}
			at := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
			mux := http.NewServeMux()
			registerCampaigns(mux, s, func() time.Time { return at }, time.Second)
			r := httptest.NewRequest("POST", "/campaigns/1/approve", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.tag != "" {
				r.Header.Set("If-Match", tc.tag)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || s.calls != tc.calls || strings.Contains(w.Body.String(), "private") {
				t.Fatal("approval contract failed", w.Code, s.calls)
			}
			if tc.calls > 0 && (!s.deadline || !s.asOf.Equal(at) || s.expected != 7 || s.fingerprint != fingerprint) {
				t.Fatal("approval input/deadline lost")
			}
			if tc.name == "approve" && w.Header().Get("ETag") != revisionTag(1, 8) {
				t.Fatal("missing new revision")
			}
			if tc.name == "changed preview" {
				var body struct {
					Error   struct{ Code, Path string }
					Preview campaign.ApprovalPreview
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Code != "approval_preview_changed" || body.Preview.Revision != 7 || body.Preview.ApprovalFingerprint == nil {
					t.Fatal("updated preview missing", err)
				}
			}
		})
	}
}

func TestDeliveryStatusHTTP(t *testing.T) {
	for _, tc := range []struct {
		path   string
		err    error
		status int
	}{
		{"/campaigns/1/delivery-status", nil, 200},
		{"/campaigns/1/delivery-status", campaign.ErrNotFound, 404},
		{"/campaigns/0/delivery-status", nil, 422},
		{"/campaigns/1/delivery-status", errors.New("private detail"), 503},
	} {
		mux := http.NewServeMux()
		s := &fakeCampaignStore{err: tc.err}
		registerCampaigns(mux, s, func() time.Time { return time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC) }, time.Second)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "private") {
			t.Fatal("delivery status failed", w.Code)
		}
	}
}
