package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetrics(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		s := &fakeCampaignStore{}
		if unavailable {
			s.err = errors.New("private-value")
		}
		mux := http.NewServeMux()
		registerCampaigns(mux, s, time.Now, time.Second)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		if !s.deadline || strings.Contains(w.Body.String(), "private-value") {
			t.Fatal("deadline or privacy failure")
		}
		if unavailable {
			if w.Code != 503 {
				t.Fatal("database error hidden")
			}
			continue
		}
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain; version=0.0.4") || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("metrics HTTP contract")
		}
		for _, want := range []string{`deliveries{channel="sms",status="sent"} 4`, `deliveries{channel="email",status="dead"} 0`, `queue_depth{channel="sms"} 3`, `llm_calls_total{call="ExtractSegment",outcome="success"} 3`, `llm_validation_failures_total{call="ExtractSegment"} 2`} {
			if !strings.Contains(w.Body.String(), want+"\n") {
				t.Fatalf("missing metric %s", want)
			}
		}
	}
}
