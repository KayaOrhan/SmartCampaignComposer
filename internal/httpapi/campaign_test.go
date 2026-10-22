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
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

type fakeCampaignStore struct {
	err      error
	calls    int
	asOf     time.Time
	deadline bool
}

func (f *fakeCampaignStore) called(ctx context.Context) { f.calls++; _, f.deadline = ctx.Deadline() }
func (f *fakeCampaignStore) Create(ctx context.Context, d campaign.Draft) (campaign.Campaign, error) {
	f.called(ctx)
	return campaign.Campaign{ID: 1, Brief: d.Brief, Channels: d.Channels, Status: "draft"}, f.err
}
func (f *fakeCampaignStore) Get(ctx context.Context, id int64) (campaign.Campaign, error) {
	f.called(ctx)
	return campaign.Campaign{ID: id}, f.err
}
func (f *fakeCampaignStore) PutSegment(ctx context.Context, id int64, v segment.Validated) (campaign.Campaign, error) {
	f.called(ctx)
	return campaign.Campaign{ID: id, Segment: v.JSON()}, f.err
}
func (f *fakeCampaignStore) Preview(ctx context.Context, id int64, at time.Time) (campaign.Preview, error) {
	f.called(ctx)
	f.asOf = at
	return campaign.Preview{AsOf: at, ByChannel: map[string]campaign.Bounds{}}, f.err
}

func TestCampaignRoutes(t *testing.T) {
	valid := `{"version":1,"all":[{"field":"city","op":"in","value":["istanbul"]}]}`
	for _, tc := range []struct {
		name, method, path, body string
		err                      error
		status, calls            int
	}{
		{"create", "POST", "/campaigns", `{"brief":"test","channels":["sms"]}`, nil, 201, 1},
		{"read", "GET", "/campaigns/1", "", nil, 200, 1},
		{"segment", "PUT", "/campaigns/1/segment", valid, nil, 200, 1},
		{"preview", "POST", "/campaigns/1/audience-preview", "", nil, 200, 1},
		{"missing", "GET", "/campaigns/1", "", campaign.ErrNotFound, 404, 1},
		{"state", "PUT", "/campaigns/1/segment", valid, campaign.ErrState, 409, 1},
		{"missing segment", "POST", "/campaigns/1/audience-preview", "", campaign.ErrSegmentRequired, 409, 1},
		{"unavailable", "GET", "/campaigns/1", "", errors.New("private database details"), 503, 1},
		{"bad id", "GET", "/campaigns/-1", "", nil, 422, 0},
		{"malformed", "POST", "/campaigns", "{", nil, 400, 0},
		{"duplicate key", "POST", "/campaigns", `{"brief":"a","brief":"b","channels":["sms"]}`, nil, 400, 0},
		{"unknown key", "POST", "/campaigns", `{"brief":"a","channels":["sms"],"status":"approved"}`, nil, 422, 0},
		{"invalid channels", "POST", "/campaigns", `{"brief":"a","channels":["push"]}`, nil, 422, 0},
		{"invalid brief", "POST", "/campaigns", `{"brief":" ","channels":["sms"]}`, nil, 422, 0},
		{"bad segment", "PUT", "/campaigns/1/segment", `{"version":1,"all":[]}`, nil, 422, 0},
		{"trailing json", "PUT", "/campaigns/1/segment", valid + ` {}`, nil, 400, 0},
		{"oversized", "POST", "/campaigns", strings.Repeat("a", segment.MaxJSONBytes+1), nil, 413, 0},
		{"preview body", "POST", "/campaigns/1/audience-preview", `{"channel":"push"}`, nil, 422, 0},
		{"wrong method", "DELETE", "/campaigns/1", "", nil, 405, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeCampaignStore{err: tc.err}
			at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
			mux := http.NewServeMux()
			registerCampaigns(mux, store, func() time.Time { return at }, time.Second)
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.status || store.calls != tc.calls {
				t.Fatalf("status=%d calls=%d body=%s", w.Code, store.calls, w.Body.String())
			}
			if tc.calls > 0 && !store.deadline {
				t.Fatal("request context has no deadline")
			}
			if strings.Contains(w.Body.String(), "private database") {
				t.Fatal("database detail leaked")
			}
			if tc.name == "create" && w.Header().Get("Location") != "/campaigns/1" {
				t.Fatal("missing resource location")
			}
			if tc.name == "preview" && !store.asOf.Equal(at) {
				t.Fatal("clock not injected")
			}
		})
	}
}

type waitingStore struct {
	fakeCampaignStore
	observed error
}

func (s *waitingStore) Get(ctx context.Context, id int64) (campaign.Campaign, error) {
	<-ctx.Done()
	s.observed = ctx.Err()
	return campaign.Campaign{}, ctx.Err()
}

func TestCampaignContextCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		s := &waitingStore{}
		mux := http.NewServeMux()
		registerCampaigns(mux, s, func() time.Time { return time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC) }, time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		if cancelled {
			cancel()
		}
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/campaigns/1", nil).WithContext(ctx)
		mux.ServeHTTP(w, r)
		cancel()
		want := context.DeadlineExceeded
		if cancelled {
			want = context.Canceled
		}
		if !errors.Is(s.observed, want) || w.Code != 503 {
			t.Fatal("request deadline/cancellation not propagated")
		}
	}
}
