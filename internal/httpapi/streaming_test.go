package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
)

type streamStore struct {
	generationStore
	preview    campaign.Preview
	previewErr error
}

func (s *streamStore) Preview(ctx context.Context, id int64, at time.Time) (campaign.Preview, error) {
	p := s.preview
	p.AsOf = at
	return p, s.previewErr
}

func (s *streamStore) GenerationInput(ctx context.Context, id int64, at time.Time) (campaign.Campaign, campaign.Preview, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return c, campaign.Preview{}, err
	}
	p, err := s.Preview(ctx, id, at)
	return c, p, err
}

type streamRecorder struct{ calls []llm.Call }

func (r *streamRecorder) RecordLLMCall(ctx context.Context, c llm.Call) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.calls = append(r.calls, c)
	return nil
}
func streamFixture() *streamStore {
	return &streamStore{generationStore: generationStore{current: campaign.Campaign{ID: 1, Revision: 1, Brief: "synthetic brief", Channels: []string{"sms", "email"}, Status: "draft", Segment: json.RawMessage(`{"version":1,"all":[{"field":"city","op":"in","value":["istanbul"]}]}`)}}, preview: campaign.Preview{EligibleUnique: 2, ByChannel: map[string]campaign.Bounds{"sms": {Count: 2, MaxFirstName: 10, MaxCity: 12}, "email": {Count: 2, MaxFirstName: 10, MaxCity: 12}}}}
}
func streamClock() time.Time { return time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC) }

func TestMessageStreamEvents(t *testing.T) {
	for _, tc := range []struct {
		scenario string
		success  bool
		attempts int
	}{
		{"happy", true, 1}, {"repair_succeeds", true, 2}, {"repair_exhausted", false, 3}, {"refusal", false, 1}, {"timeout", false, 1}, {"rate_limited", false, 2},
	} {
		t.Run(tc.scenario, func(t *testing.T) {
			s := streamFixture()
			r := &streamRecorder{}
			mock, err := llm.NewMessageMock(tc.scenario, 0)
			if err != nil {
				t.Fatal(err)
			}
			timeout := time.Second
			if tc.scenario == "timeout" {
				timeout = 5 * time.Millisecond
			}
			g := llm.MessageGenerator{Provider: mock, Recorder: r, Timeout: timeout}
			mux := http.NewServeMux()
			registerMessageGeneration(mux, s, g, streamClock, 2*time.Second)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/campaigns/1/generate-messages", nil))
			if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("SSE headers/status")
			}
			attempt, finals, failures := 0, 0, 0
			text := map[string]string{}
			frames := strings.Split(strings.TrimSpace(w.Body.String()), "\n\n")
			for i, frame := range frames {
				lines := strings.Split(frame, "\n")
				if len(lines) != 2 {
					t.Fatal("invalid SSE framing")
				}
				kind := strings.TrimPrefix(lines[0], "event: ")
				data := []byte(strings.TrimPrefix(lines[1], "data: "))
				var event struct {
					Attempt              int
					Channel, Field, Text string
					Messages             map[string]map[string]string
					BaseRevision         int64 `json:"base_revision"`
					Audience             campaign.Preview
					Error                *llm.Error
				}
				if err := json.Unmarshal(data, &event); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "attempt":
					if event.Attempt != attempt+1 {
						t.Fatal("attempt order")
					}
					attempt = event.Attempt
					text = map[string]string{}
				case "delta":
					if event.Attempt != attempt || finals != 0 || failures != 0 {
						t.Fatal("delta order")
					}
					text[event.Channel+"."+event.Field] += event.Text
				case "validated":
					finals++
					if i != len(frames)-1 || event.Attempt != attempt || event.BaseRevision != 1 || !event.Audience.AsOf.Equal(streamClock()) {
						t.Fatal("final order or preview clock")
					}
					for channel, fields := range event.Messages {
						for field, value := range fields {
							if text[channel+"."+field] != value {
								t.Fatal("deltas differ from validated payload")
							}
						}
					}
				case "error":
					failures++
					if i != len(frames)-1 || event.Error == nil {
						t.Fatal("safe terminal error missing")
					}
				default:
					t.Fatal("unknown event")
				}
			}
			if attempt != tc.attempts || len(r.calls) != tc.attempts || (tc.success && (finals != 1 || failures != 0)) || (!tc.success && (finals != 0 || failures != 1)) {
				t.Fatalf("attempt=%d validated=%d error=%d", attempt, finals, failures)
			}
			if s.puts != 0 || s.current.Status != "draft" {
				t.Fatal("stream persisted messages or state")
			}
		})
	}
}

func TestMessageStreamPreflight(t *testing.T) {
	for _, tc := range []struct {
		name, body, state  string
		missingSegment     bool
		getErr, previewErr error
		status             int
	}{
		{name: "body", body: `{"tone":"friendly"}`, status: 422},
		{name: "oversized", body: strings.Repeat(" ", 32769), status: 413},
		{name: "state", state: "approved", status: 409},
		{name: "segment required", missingSegment: true, status: 409},
		{name: "missing campaign", getErr: campaign.ErrNotFound, status: 404},
		{name: "preview failure", previewErr: errors.New("private database details"), status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := streamFixture()
			s.getErr = tc.getErr
			s.previewErr = tc.previewErr
			if tc.state != "" {
				s.current.Status = tc.state
			}
			if tc.missingSegment {
				s.current.Segment = nil
			}
			mock, _ := llm.NewMessageMock("happy", 0)
			r := &streamRecorder{}
			mux := http.NewServeMux()
			registerMessageGeneration(mux, s, llm.MessageGenerator{Provider: mock, Recorder: r, Timeout: time.Second}, streamClock, time.Second)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/campaigns/1/generate-messages", strings.NewReader(tc.body)))
			if w.Code != tc.status || strings.Contains(w.Body.String(), "private") || len(r.calls) != 0 || strings.Contains(w.Body.String(), "event:") {
				t.Fatal("preflight failed", w.Code)
			}
		})
	}
}

type blockingMessageProvider struct {
	release   <-chan struct{}
	cancelled chan<- struct{}
}

func (p blockingMessageProvider) GenerateMessages(ctx context.Context, req llm.Request, emit func(string, string, string) error) (llm.Response, error) {
	if err := emit("sms", "body", "Hi"); err != nil {
		return llm.Response{}, err
	}
	select {
	case <-ctx.Done():
		if p.cancelled != nil {
			close(p.cancelled)
		}
		return llm.Response{}, ctx.Err()
	case <-p.release:
		return llm.Response{Tool: "generate_messages", Input: json.RawMessage(`{"sms":{"body":"Hi"},"email":{"subject":"Hello","body":"Body"}}`)}, nil
	}
}

func TestSSEFlushesBeforeGenerationCompletes(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	s := streamFixture()
	mux := http.NewServeMux()
	g := llm.MessageGenerator{Provider: blockingMessageProvider{release: release}, Recorder: &streamRecorder{}, Timeout: 3 * time.Second}
	registerMessageGeneration(mux, s, g, streamClock, 3*time.Second)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Post(server.URL+"/campaigns/1/generate-messages", "", nil)
	if err != nil {
		t.Fatal("headers buffered until completion", err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	sawDelta := false
	for scanner.Scan() {
		if scanner.Text() == "event: delta" {
			sawDelta = true
			break
		}
	}
	if !sawDelta {
		t.Fatal("delta not flushed while provider was blocked", scanner.Err())
	}
}

func TestSSEDisconnectCancelsProvider(t *testing.T) {
	cancelled := make(chan struct{})
	s := streamFixture()
	mux := http.NewServeMux()
	g := llm.MessageGenerator{Provider: blockingMessageProvider{cancelled: cancelled}, Recorder: &streamRecorder{}, Timeout: 3 * time.Second}
	registerMessageGeneration(mux, s, g, streamClock, 3*time.Second)
	server := httptest.NewServer(mux)
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Post(server.URL+"/campaigns/1/generate-messages", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if scanner.Text() == "event: delta" {
			break
		}
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("disconnect did not cancel provider")
	}
}

type failedStreamWriter struct {
	header http.Header
	writes int
}

func (w *failedStreamWriter) Header() http.Header { return w.header }
func (w *failedStreamWriter) WriteHeader(int)     {}
func (w *failedStreamWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("private transport failure")
}
func (w *failedStreamWriter) Flush() {}
func TestSSEWriteFailureStopsGeneration(t *testing.T) {
	s := streamFixture()
	mock, _ := llm.NewMessageMock("happy", 0)
	r := &streamRecorder{}
	mux := http.NewServeMux()
	registerMessageGeneration(mux, s, llm.MessageGenerator{Provider: mock, Recorder: r, Timeout: time.Second}, streamClock, time.Second)
	w := &failedStreamWriter{header: http.Header{}}
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/campaigns/1/generate-messages", nil))
	if w.writes > 2 || s.puts != 0 {
		t.Fatal("stream continued after failed write")
	}
}
