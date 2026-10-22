package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type generationStore struct {
	fakeCampaignStore
	current        campaign.Campaign
	getErr, putErr error
	puts           int
}

func (s *generationStore) Get(ctx context.Context, id int64) (campaign.Campaign, error) {
	return s.current, s.getErr
}
func (s *generationStore) PutSegment(ctx context.Context, id int64, v segment.Validated) (campaign.Campaign, error) {
	s.puts++
	if s.putErr != nil {
		return campaign.Campaign{}, s.putErr
	}
	if ctx.Err() != nil {
		return campaign.Campaign{}, ctx.Err()
	}
	s.current.Segment = v.JSON()
	return s.current, nil
}

type generationFunc func(context.Context, int64, string) (segment.Validated, error)

func (f generationFunc) ExtractSegment(ctx context.Context, id int64, brief string) (segment.Validated, error) {
	return f(ctx, id, brief)
}

func TestGenerateSegmentEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name, body, state           string
		getErr, generateErr, putErr error
		status, called, puts        int
	}{
		{name: "happy", status: 200, called: 1, puts: 1},
		{name: "body", body: `{"scenario":"happy"}`, status: 422},
		{name: "oversized", body: strings.Repeat(" ", segment.MaxJSONBytes+1), status: 413},
		{name: "missing", getErr: campaign.ErrNotFound, status: 404},
		{name: "not draft", state: "approved", status: 409},
		{name: "state changed during generation", putErr: campaign.ErrState, status: 409, called: 1, puts: 1},
		{name: "invalid", generateErr: &llm.Error{Code: "llm_output_invalid", Path: "$", Reasons: []segment.ValidationError{{Code: "unsupported_field", Path: "all[0].field"}}}, status: 502, called: 1},
		{name: "refusal", generateErr: &llm.Error{Code: "llm_refused", Path: "$"}, status: 502, called: 1},
		{name: "timeout", generateErr: &llm.Error{Code: "llm_timeout", Path: "$"}, status: 504, called: 1},
		{name: "unavailable", generateErr: &llm.Error{Code: "llm_unavailable", Path: "$"}, status: 503, called: 1},
		{name: "audit unavailable", generateErr: &llm.Error{Code: "llm_audit_unavailable", Path: "$"}, status: 503, called: 1},
		{name: "database unavailable", getErr: errors.New("private details"), status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := []byte(`{"version":1,"all":[{"field":"language","op":"in","value":["tr"]}]}`)
			state := tc.state
			if state == "" {
				state = "draft"
			}
			s := &generationStore{current: campaign.Campaign{ID: 1, Brief: "stored brief", Channels: []string{"sms"}, Status: state, Segment: old}, getErr: tc.getErr, putErr: tc.putErr}
			calls := 0
			g := generationFunc(func(ctx context.Context, id int64, brief string) (segment.Validated, error) {
				calls++
				if _, ok := ctx.Deadline(); !ok || id != 1 || brief != "stored brief" {
					t.Fatal("wrong generation input or missing deadline")
				}
				if tc.generateErr != nil {
					return segment.Validated{}, tc.generateErr
				}
				return segment.Parse([]byte(`{"version":1,"all":[{"field":"city","op":"in","value":["Istanbul"]}]}`))
			})
			mux := http.NewServeMux()
			registerGeneration(mux, s, g, time.Second)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/campaigns/1/generate-segment", strings.NewReader(tc.body)))
			if w.Code != tc.status || calls != tc.called || s.puts != tc.puts {
				t.Fatalf("status=%d calls=%d puts=%d", w.Code, calls, s.puts)
			}
			if tc.status != 200 && string(s.current.Segment) != string(old) {
				t.Fatal("failed generation changed saved segment")
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Fatal("private error leaked")
			}
			if tc.status == 200 && !strings.Contains(w.Body.String(), "istanbul") {
				t.Fatal("canonical generated segment missing")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable generation result")
			}
		})
	}
}

func TestGenerateTimeoutDoesNotSave(t *testing.T) {
	s := &generationStore{current: campaign.Campaign{ID: 1, Brief: "brief", Status: "draft"}}
	g := generationFunc(func(ctx context.Context, id int64, brief string) (segment.Validated, error) {
		<-ctx.Done()
		return segment.Validated{}, &llm.Error{Code: "llm_timeout", Path: "$"}
	})
	mux := http.NewServeMux()
	registerGeneration(mux, s, g, time.Millisecond)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/campaigns/1/generate-segment", nil))
	if w.Code != 504 || s.puts != 0 {
		t.Fatal("timed out generation persisted")
	}
}
