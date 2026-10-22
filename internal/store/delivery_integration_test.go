//go:build integration

package store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/mockprovider"
)

func queuedFixture(t *testing.T) (Campaigns, Deliveries, context.Context, campaign.Campaign) {
	t.Helper()
	repo, ctx, c, at := approvalFixture(t)
	p, err := repo.ApprovalPreview(ctx, c.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at); err != nil {
		t.Fatal(err)
	}
	c, err = repo.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, Deliveries{DB: repo.DB, Timeout: 2 * time.Second}, ctx, c
}

func mockClient(t *testing.T, p mockprovider.Provider) *delivery.Client {
	t.Helper()
	server := httptest.NewServer(p.Handler())
	t.Cleanup(server.Close)
	client, err := delivery.NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func expire(t *testing.T, q Deliveries, ctx context.Context, id int64) {
	t.Helper()
	if _, err := q.DB.ExecContext(ctx, `UPDATE app.deliveries SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, id); err != nil {
		t.Fatal("expire failed")
	}
	if err := q.Recover(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestDeliverySuccessAndCompletion(t *testing.T) {
	repo, q, ctx, c := queuedFixture(t)
	client := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	for _, ch := range []string{"sms", "email"} {
		for {
			j, err := q.Claim(ctx, ch, 30*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if j.ID == 0 {
				break
			}
			outcome, err := q.Dispatch(ctx, j, 30*time.Second, client.Send)
			if err != nil || outcome.Status != "sent" {
				t.Fatal("dispatch failed", err)
			}
			if err = q.Finish(ctx, j, outcome, 0); err != nil {
				t.Fatal(err)
			}
		}
	}
	status, err := repo.GetDeliveryStatus(ctx, c.ID)
	if err != nil || status.Status != "completed" || status.Counts.Sent != 3 || status.Revision != c.Revision {
		t.Fatal("completion changed revision/counts", err)
	}
	var accepted int
	if q.DB.QueryRowContext(ctx, `SELECT count(*) FROM app.mock_acceptances`).Scan(&accepted) != nil || accepted != 3 {
		t.Fatal("provider count mismatch")
	}
}

func TestDeliveryFailureLifecycle(t *testing.T) {
	for _, scenario := range []string{"transient", "permanent", "exhausted"} {
		t.Run(scenario, func(t *testing.T) {
			_, q, ctx, _ := queuedFixture(t)
			provider := mockprovider.Provider{DB: q.DB, Timeout: time.Second, Rate5xx: 1}
			var permanent *httptest.Server
			client := mockClient(t, provider)
			if scenario == "permanent" {
				permanent = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(422) }))
				defer permanent.Close()
				var err error
				client, err = delivery.NewClient(permanent.URL, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
			}
			for attempt := 1; attempt <= 5; attempt++ {
				j, err := q.Claim(ctx, "sms", 30*time.Second)
				if err != nil || j.ID == 0 || j.Attempts != attempt {
					t.Fatal("attempt mismatch", err)
				}
				out, err := q.Dispatch(ctx, j, 30*time.Second, client.Send)
				if err != nil {
					t.Fatal(err)
				}
				expected := "pending"
				if scenario == "permanent" {
					expected = "dead"
				}
				if scenario == "transient" && attempt == 2 {
					expected = "sent"
				}
				if out.Status != expected {
					t.Fatal("incorrect classification", out.Status)
				}
				if out.Status == "pending" && attempt == 5 {
					out.Status = "dead"
				}
				if err = q.Finish(ctx, j, out, time.Hour); err != nil {
					t.Fatal(err)
				}
				if out.Status != "pending" {
					break
				}
				if next, err := q.Claim(ctx, "sms", 30*time.Second); err != nil || next.ID != 0 {
					t.Fatal("backoff ignored", err)
				}
				if _, err = q.DB.ExecContext(ctx, `UPDATE app.deliveries SET next_attempt_at=clock_timestamp() WHERE id=$1`, j.ID); err != nil {
					t.Fatal(err)
				}
				if scenario == "transient" {
					client = mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
				}
			}
			var state string
			var attempts int
			if err := q.DB.QueryRowContext(ctx, `SELECT status,attempts FROM app.deliveries WHERE channel='sms'`).Scan(&state, &attempts); err != nil {
				t.Fatal(err)
			}
			if scenario == "exhausted" && (state != "dead" || attempts != 5) {
				t.Fatal("retry cap lost")
			}
			if scenario == "permanent" && attempts != 1 {
				t.Fatal("permanent failure retried")
			}
		})
	}
}

func TestDeliveryCrashFencingAndProviderRestart(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	first, err := q.Claim(ctx, "sms", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	accepted, err := q.Dispatch(ctx, first, 30*time.Second, client.Send)
	if err != nil || accepted.Status != "sent" {
		t.Fatal("first acceptance failed", err)
	}
	// Crash after acceptance, before the database acknowledgement.
	expire(t, q, ctx, first.ID)
	second, err := q.Claim(ctx, "sms", 30*time.Second)
	if err != nil || second.ID != first.ID || second.Generation <= first.Generation {
		t.Fatal("lease recovery failed", err)
	}
	if err = q.Finish(ctx, first, accepted, 0); !errors.Is(err, delivery.ErrLostClaim) {
		t.Fatal("old worker not fenced", err)
	}
	if _, err = q.Dispatch(ctx, first, 30*time.Second, func(context.Context, delivery.Job) delivery.Outcome { t.Error("stale owner sent"); return accepted }); !errors.Is(err, delivery.ErrLostClaim) {
		t.Fatal("stale dispatch not fenced", err)
	}
	restarted := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	repeated, err := q.Dispatch(ctx, second, 30*time.Second, restarted.Send)
	if err != nil || repeated.MessageID != accepted.MessageID {
		t.Fatal("provider restart lost dedupe", err)
	}
	if err = q.Finish(ctx, second, repeated, 0); err != nil {
		t.Fatal(err)
	}
	var n int
	if q.DB.QueryRowContext(ctx, `SELECT count(*) FROM app.mock_acceptances`).Scan(&n) != nil || n != 1 {
		t.Fatal("duplicate acceptance")
	}
	// Same key, different content must never be accepted.
	second.Payload.Body = "Different"
	if out := restarted.Send(ctx, second); out.Status != "dead" {
		t.Fatal("idempotency payload mismatch accepted")
	}
}

func TestConcurrentClaimsAndWorkers(t *testing.T) {
	repo, q, ctx, c := queuedFixture(t)
	jobs := make(chan delivery.Job, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { j, err := q.Claim(ctx, "sms", 30*time.Second); jobs <- j; errs <- err })
	}
	wg.Wait()
	close(jobs)
	wins := 0
	var claimed delivery.Job
	for j := range jobs {
		if j.ID != 0 {
			wins++
			claimed = j
		}
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatal("same active job claimed twice")
	}
	expire(t, q, ctx, claimed.ID)
	client := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	workerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 2)
	for range 2 {
		go func() {
			done <- (delivery.Worker{Queue: q, Send: client.Send, Concurrency: 2, Lease: 30 * time.Second, Poll: time.Millisecond}).Run(workerCtx)
		}()
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	complete := false
	for !complete {
		select {
		case <-workerCtx.Done():
			t.Fatal("workers did not complete")
		case <-ticker.C:
			status, err := repo.GetDeliveryStatus(ctx, c.ID)
			if err != nil {
				t.Fatal(err)
			}
			complete = status.Status == "completed"
		}
	}
	cancel()
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	status, err := repo.GetDeliveryStatus(ctx, c.ID)
	if err != nil || status.Counts.Sent != 3 || status.Revision != c.Revision {
		t.Fatal("concurrent results wrong", err)
	}
	var n int
	if q.DB.QueryRowContext(ctx, `SELECT count(*) FROM app.mock_acceptances`).Scan(&n) != nil || n != 3 {
		t.Fatal("duplicate worker effects")
	}
}

func TestDeliveryDatabaseFailuresAndLeaseExhaustion(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	if _, err := q.DB.ExecContext(ctx, `ALTER TABLE app.deliveries ADD CONSTRAINT reject_claim CHECK(status<>'in_flight')`); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Claim(ctx, "sms", 30*time.Second); err == nil {
		t.Fatal("claim failure hidden")
	}
	if _, err := q.DB.ExecContext(ctx, `ALTER TABLE app.deliveries DROP CONSTRAINT reject_claim`); err != nil {
		t.Fatal(err)
	}
	j, err := q.Claim(ctx, "sms", 30*time.Second)
	if err != nil || j.Attempts != 1 {
		t.Fatal("claim failure consumed attempt", err)
	}
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.deliveries ADD CONSTRAINT reject_result CHECK(status<>'sent')`); err != nil {
		t.Fatal(err)
	}
	if err = q.Finish(ctx, j, delivery.Outcome{Status: "sent"}, 0); err == nil {
		t.Fatal("result failure hidden")
	}
	var state string
	if q.DB.QueryRowContext(ctx, `SELECT status FROM app.deliveries WHERE id=$1`, j.ID).Scan(&state) != nil || state != "in_flight" {
		t.Fatal("result not rolled back")
	}
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.deliveries DROP CONSTRAINT reject_result; UPDATE app.deliveries SET attempts=5 WHERE status='in_flight'`); err != nil {
		t.Fatal(err)
	}
	expire(t, q, ctx, j.ID)
	if next, err := q.Claim(ctx, "sms", 30*time.Second); err != nil || next.ID != 0 {
		t.Fatal("exhausted crash retried", err)
	}
	if q.DB.QueryRowContext(ctx, `SELECT status FROM app.deliveries WHERE id=$1`, j.ID).Scan(&state) != nil || state != "dead" {
		t.Fatal("crash exhaustion not terminal")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = q.Claim(cancelled, "email", time.Second); err == nil {
		t.Fatal("cancelled transaction succeeded")
	}
}

func TestFinalConsentAfterGateWait(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	j, err := q.Claim(ctx, "sms", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := q.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer gate.Close()
	if _, err = gate.ExecContext(ctx, `SELECT pg_advisory_lock(730001)`); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	finished := make(chan delivery.Outcome, 1)
	failed := make(chan error, 1)
	go func() {
		out, err := q.Dispatch(ctx, j, 30*time.Second, func(context.Context, delivery.Job) delivery.Outcome {
			calls.Add(1)
			return delivery.Outcome{Status: "sent"}
		})
		finished <- out
		failed <- err
	}()
	if _, err = q.DB.ExecContext(ctx, `UPDATE app.consents SET granted=false WHERE user_id=1 AND channel='sms'`); err != nil {
		t.Fatal(err)
	}
	if _, err = gate.ExecContext(ctx, `SELECT pg_advisory_unlock(730001)`); err != nil {
		t.Fatal(err)
	}
	out := <-finished
	if err := <-failed; err != nil || out.Status != "skipped" || calls.Load() != 0 {
		t.Fatal("revocation during wait ignored", err)
	}
	if err = q.Finish(ctx, j, out, 0); err != nil {
		t.Fatal(err)
	}
}

func TestSharedChannelSpacingAndProviderLimit(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	first, err := q.Claim(ctx, "email", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := q.Claim(ctx, "email", 30*time.Second)
	if err != nil || second.ID == 0 {
		t.Fatal("second job missing", err)
	}
	starts := make(chan time.Time, 2)
	errs := make(chan error, 2)
	send := func(ctx context.Context, j delivery.Job) delivery.Outcome {
		var at time.Time
		if err := q.DB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			t.Error("database clock unavailable")
			starts <- time.Time{}
			return delivery.Outcome{Status: "pending"}
		}
		starts <- at
		return delivery.Outcome{Status: "sent"}
	}
	for _, j := range []delivery.Job{first, second} {
		go func() { _, err := q.Dispatch(ctx, j, 30*time.Second, send); errs <- err }()
	}
	a, b := <-starts, <-starts
	if b.Sub(a) < 10*time.Millisecond {
		t.Fatal("shared channel gate permitted a burst")
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if _, err = q.DB.ExecContext(ctx, `INSERT INTO app.mock_calls(channel) SELECT 'email' FROM generate_series(1,100)`); err != nil {
		t.Fatal(err)
	}
	client := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	out := client.Send(ctx, first)
	if out.Status != "pending" || out.RetryAfter != time.Second {
		t.Fatal("independent provider rate limit failed")
	}
}

func TestConsentAndProviderStorageFailClosed(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	j, err := q.Claim(ctx, "sms", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.consents RENAME TO unavailable_consents`); err != nil {
		t.Fatal(err)
	}
	_, err = q.Dispatch(ctx, j, 30*time.Second, func(context.Context, delivery.Job) delivery.Outcome {
		t.Error("consent failure sent")
		return delivery.Outcome{Status: "sent"}
	})
	if err == nil {
		t.Fatal("consent query failure accepted")
	}
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.unavailable_consents RENAME TO consents`); err != nil {
		t.Fatal(err)
	}
	client := mockClient(t, mockprovider.Provider{DB: q.DB, Timeout: time.Second})
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.mock_acceptances ADD CONSTRAINT reject_acceptance CHECK(false)`); err != nil {
		t.Fatal(err)
	}
	if out := client.Send(ctx, j); out.Status != "pending" {
		t.Fatal("provider transaction failure not retryable")
	}
	if _, err = q.DB.ExecContext(ctx, `ALTER TABLE app.mock_acceptances DROP CONSTRAINT reject_acceptance`); err != nil {
		t.Fatal(err)
	}
	if out := client.Send(ctx, j); out.Status != "sent" {
		t.Fatal("provider transaction could not recover")
	}
}

func TestSharedRollingWindowAndWaitingLeaseRenewal(t *testing.T) {
	_, q, ctx, _ := queuedFixture(t)
	j, err := q.Claim(ctx, "sms", 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.DB.ExecContext(ctx, `UPDATE app.delivery_channels SET next_start_at=clock_timestamp()+interval '450 milliseconds' WHERE channel='sms'`); err != nil {
		t.Fatal(err)
	}
	record := func(ctx context.Context, j delivery.Job) delivery.Outcome { return delivery.Outcome{Status: "sent"} }
	if _, err = q.Dispatch(ctx, j, 300*time.Millisecond, record); err != nil {
		t.Fatal("waiting lease was not renewed", err)
	}
	// Measure the database-authoritative gate, including concurrent callers and
	// repeat dispatch. Expire/reclaim uses SQL time rather than wall-clock sleeps.
	var starts []time.Time
	var mu sync.Mutex
	var wg sync.WaitGroup
	record = func(ctx context.Context, j delivery.Job) delivery.Outcome {
		var at time.Time
		if err := q.DB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			t.Error("clock query failed")
			return delivery.Outcome{Status: "pending"}
		}
		mu.Lock()
		starts = append(starts, at)
		mu.Unlock()
		return delivery.Outcome{Status: "sent"}
	}
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			for range 26 {
				if _, err := q.Dispatch(ctx, j, 30*time.Second, record); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		})
	}
	wg.Wait()
	for range 4 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(starts, func(i, k int) bool { return starts[i].Before(starts[k]) })
	for i := 100; i < len(starts); i++ {
		if starts[i].Sub(starts[i-100]) < time.Second {
			t.Fatal("more than 100 channel admissions in a rolling second")
		}
	}
	if len(starts) != 104 {
		t.Fatal("missing admission samples")
	}
	if _, err = q.DB.ExecContext(ctx, `UPDATE app.delivery_channels SET next_start_at=clock_timestamp()+interval '1 hour' WHERE channel='sms'`); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = q.Dispatch(cancelled, j, time.Second, func(context.Context, delivery.Job) delivery.Outcome {
		t.Error("cancelled gate sent")
		return delivery.Outcome{Status: "sent"}
	}); err == nil {
		t.Fatal("admission ignored cancellation")
	}
}
