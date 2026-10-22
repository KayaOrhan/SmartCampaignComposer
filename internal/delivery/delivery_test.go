package delivery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRetryPolicyAndMockClient(t *testing.T) {
	for _, status := range []int{200, 400, 401, 409, 422, 429, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Idempotency-Key") != "campaign:1:user:1:sms" || r.Method != "POST" {
					t.Error("request contract")
				}
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"message_id":"mock-%s"}`, strings.Repeat("a", 64))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			out := client.Send(context.Background(), Job{Key: "campaign:1:user:1:sms"})
			expected := "dead"
			if status == 200 {
				expected = "sent"
			}
			if status == 429 || status >= 500 {
				expected = "pending"
			}
			if out.Status != expected {
				t.Fatal("classification", out.Status)
			}
			if expected == "pending" && out.RetryAfter != 3*time.Second {
				t.Fatal("Retry-After lost")
			}
		})
	}
	for _, base := range []string{"https://mockprovider:8081", "http://example.com:80", "http://mockprovider:8081/x", "http://user:pass@mockprovider:8081", "http://mockprovider:8081?x=y"} {
		if _, err := NewClient(base, time.Second); err == nil {
			t.Fatal("unsafe host accepted")
		}
	}
	for _, attempt := range []int{1, 2, 5, 100} {
		if d := RetryDelay(attempt, 7*time.Second, func(int64) int64 { return 0 }); d != 7*time.Second {
			t.Fatal("minimum retry delay")
		}
		if d := RetryDelay(attempt, 0, func(n int64) int64 { return n - 1 }); d > 60*time.Second {
			t.Fatal("unbounded retry")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Wait(ctx, time.Hour) == nil {
		t.Fatal("wait ignored cancellation")
	}
}

func TestClientProtocolRedirectAndCancellation(t *testing.T) {
	for _, body := range []string{`{}`, `{"message_id":"private recipient detail"}`, strings.Repeat("x", 4097)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		c, err := NewClient(server.URL, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if o := c.Send(context.Background(), Job{}); o.Status != "pending" || o.Code != "provider_protocol" {
			t.Fatal("bad protocol accepted")
		}
		c.Close()
		server.Close()
	}
	reached := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		reached <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, time.Second)
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Outcome, 1)
	go func() { done <- c.Send(ctx, Job{}) }()
	<-reached
	cancel()
	if out := <-done; out.Status != "pending" {
		t.Fatal("cancelled request succeeded")
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://example.com", 302) }))
	defer redirect.Close()
	c2, _ := NewClient(redirect.URL, time.Second)
	defer c2.Close()
	if out := c2.Send(context.Background(), Job{}); out.Status != "dead" {
		t.Fatal("redirect followed")
	}
}

type cancelledQueue struct {
	mu       sync.Mutex
	claims   int
	finishes int
	started  chan struct{}
	once     sync.Once
}

func (q *cancelledQueue) Recover(context.Context) error { return nil }
func (q *cancelledQueue) Claim(ctx context.Context, ch string, lease time.Duration) (Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.claims++
	return Job{ID: 1, Payload: Payload{Channel: ch}}, nil
}
func (q *cancelledQueue) Dispatch(ctx context.Context, j Job, l time.Duration, send func(context.Context, Job) Outcome) (Outcome, error) {
	q.once.Do(func() { close(q.started) })
	<-ctx.Done()
	return Outcome{}, ctx.Err()
}
func (q *cancelledQueue) Finish(context.Context, Job, Outcome, time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finishes++
	return fmt.Errorf("unexpected acknowledgement")
}
func TestWorkerGracefulCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	q := &cancelledQueue{started: make(chan struct{})}
	w := Worker{Queue: q, Send: func(context.Context, Job) Outcome { t.Error("send called"); return Outcome{} }, Concurrency: 2, Lease: time.Second, Poll: time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	<-q.started
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if q.finishes != 0 {
		t.Fatal("cancelled ambiguous work was acknowledged")
	}
	if err := (Worker{}).Run(ctx); err == nil {
		t.Fatal("invalid worker accepted")
	}
}
