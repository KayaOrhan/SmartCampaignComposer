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

type editingStore struct {
	fakeCampaignStore
	raw      []byte
	revision int64
	at       time.Time
}

func (s *editingStore) PutMessages(ctx context.Context, id, expected int64, body []byte, at time.Time) (campaign.SavedMessages, error) {
	s.called(ctx)
	s.raw = body
	s.revision = expected
	s.at = at
	return campaign.SavedMessages{CampaignID: id, Revision: expected + 1, Status: "content_ready", Messages: json.RawMessage(`{"sms":{"body":"Saved"}}`)}, s.err
}

func TestMessagePersistenceHTTP(t *testing.T) {
	for _, tc := range []struct {
		name, method, body, tag string
		err                     error
		status, calls           int
	}{
		{"get", "GET", "", "", nil, 200, 1},
		{"save", "PUT", `{"messages":{"sms":{"body":"Saved"}}}`, `"campaign-1-r7"`, nil, 200, 1},
		{"missing precondition", "PUT", `{"messages":{}}`, "", nil, 428, 0},
		{"malformed precondition", "PUT", `{"messages":{}}`, "*", nil, 400, 0},
		{"unknown envelope", "PUT", `{"messages":{},"status":"approved"}`, `"campaign-1-r7"`, nil, 422, 0},
		{"duplicate messages", "PUT", `{"messages":{},"messages":{}}`, `"campaign-1-r7"`, nil, 400, 0},
		{"stale", "PUT", `{"messages":{}}`, `"campaign-1-r7"`, campaign.ErrRevision, 412, 1},
		{"concurrent", "PUT", `{"messages":{}}`, `"campaign-1-r7"`, campaign.ErrConcurrentUpdate, 409, 1},
		{"state", "PUT", `{"messages":{}}`, `"campaign-1-r7"`, campaign.ErrState, 409, 1},
		{"missing", "GET", "", "", campaign.ErrNotFound, 404, 1},
		{"database", "GET", "", "", errors.New("private details"), 503, 1},
		{"oversized", "PUT", strings.Repeat(" ", 32769), `"campaign-1-r7"`, nil, 413, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &editingStore{fakeCampaignStore: fakeCampaignStore{err: tc.err}}
			at := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
			mux := http.NewServeMux()
			registerCampaigns(mux, s, func() time.Time { return at }, time.Second)
			r := httptest.NewRequest(tc.method, "/campaigns/1/messages", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			if tc.tag != "" {
				r.Header.Set("If-Match", tc.tag)
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || s.calls != tc.calls || strings.Contains(w.Body.String(), "private") {
				t.Fatal("wrong response", w.Code, s.calls, w.Body.String())
			}
			if tc.name == "save" && (s.revision != 7 || !s.at.Equal(at) || string(s.raw) != `{"sms":{"body":"Saved"}}` || w.Header().Get("ETag") != revisionTag(1, 8) || !s.deadline) {
				t.Fatal("save input/deadline/revision lost")
			}
		})
	}
}
