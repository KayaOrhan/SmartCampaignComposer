package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEndpoints(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		dbErr              error
		status             int
		body               string
		calls              int
	}{
		{"liveness does not query database", "GET", "/health", errors.New("offline"), 200, `{"status":"ok"}`, 0},
		{"ready", "GET", "/ready", nil, 200, `{"status":"ready"}`, 1},
		{"unavailable hides database details", "GET", "/ready", errors.New("sensitive database details"), 503, `{"status":"unavailable"}`, 1},
		{"unknown", "GET", "/missing", nil, 404, "", 0},
		{"nested path", "GET", "/health/extra", nil, 404, "", 0},
		{"wrong method", "POST", "/health", nil, 405, "", 0},
		{"ready wrong method", "POST", "/ready", nil, 405, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			h := Handler(func(ctx context.Context) error {
				calls++
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing database deadline")
				}
				return tc.dbErr
			}, time.Second, nil, nil, time.Second)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d", w.Code, tc.status)
			}
			if calls != tc.calls {
				t.Fatalf("database calls = %d, want %d", calls, tc.calls)
			}
			if tc.body != "" {
				if strings.TrimSpace(w.Body.String()) != tc.body {
					t.Fatalf("body = %q", w.Body.String())
				}
				if w.Header().Get("Content-Type") != "application/json" {
					t.Fatal("expected JSON")
				}
			}
		})
	}
}

func TestReadinessTimeoutAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "cancellation"}[cancelled], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			var got error
			h := Handler(func(ctx context.Context) error { <-ctx.Done(); got = ctx.Err(); return got }, time.Millisecond, nil, nil, time.Second)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ready", nil).WithContext(ctx))
			want := context.DeadlineExceeded
			if cancelled {
				want = context.Canceled
			}
			if !errors.Is(got, want) || w.Code != 503 {
				t.Fatalf("error=%v, status=%d", got, w.Code)
			}
		})
	}
}
