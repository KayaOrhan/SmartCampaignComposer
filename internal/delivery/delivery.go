package delivery

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

var ErrUnavailable = errors.New("delivery storage unavailable")
var ErrLostClaim = errors.New("delivery claim lost")

const MaxAttempts = 5

type Payload struct {
	Channel     string  `json:"channel"`
	Destination string  `json:"destination"`
	Subject     *string `json:"subject"`
	Body        string  `json:"body"`
}

type Job struct {
	ID, CampaignID, UserID, Generation int64
	Attempts                           int
	Key                                string
	Payload                            Payload
}

type Outcome struct {
	Status, Code, MessageID string
	RetryAfter              time.Duration
}

// RetryDelay uses full jitter. Retry-After is a lower bound, never a shorter retry.
func RetryDelay(attempt int, retryAfter time.Duration, random func(int64) int64) time.Duration {
	cap := min(60*time.Second, time.Second*time.Duration(1<<min(max(attempt, 0), 6)))
	return max(time.Duration(random(int64(cap)+1)), retryAfter)
}

func Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type Queue interface {
	Claim(context.Context, string, time.Duration) (Job, error)
	Recover(context.Context) error
	Dispatch(context.Context, Job, time.Duration, func(context.Context, Job) Outcome) (Outcome, error)
	Finish(context.Context, Job, Outcome, time.Duration) error
}

type Worker struct {
	Queue       Queue
	Send        func(context.Context, Job) Outcome
	Concurrency int
	Lease, Poll time.Duration
	OnError     func(error)
}

func (w Worker) Run(ctx context.Context) error {
	if w.Concurrency < 1 || w.Concurrency > 32 || w.Lease <= 0 || w.Poll <= 0 || w.Queue == nil || w.Send == nil {
		return errors.New("invalid worker configuration")
	}
	done := make(chan struct{}, w.Concurrency*2)
	for _, channel := range []string{"sms", "email"} {
		for range w.Concurrency {
			go func() {
				defer func() { done <- struct{}{} }()
				for ctx.Err() == nil {
					if err := w.Queue.Recover(ctx); err != nil {
						w.report(err)
						if Wait(ctx, w.Poll) != nil {
							return
						}
						continue
					}
					job, err := w.Queue.Claim(ctx, channel, w.Lease)
					if err != nil {
						w.report(err)
					}
					if err != nil || job.ID == 0 {
						if Wait(ctx, w.Poll) != nil {
							return
						}
						continue
					}
					result, err := w.Queue.Dispatch(ctx, job, w.Lease, w.Send)
					if ctx.Err() != nil {
						return
					} // ambiguous calls remain leased for recovery/deduplication.
					if err != nil {
						w.report(err)
						if errors.Is(err, ErrLostClaim) {
							continue
						}
						result = Outcome{Status: "pending", Code: "dispatch_unavailable"}
					}
					if result.Status == "pending" && job.Attempts >= MaxAttempts {
						result.Status = "dead"
					}
					delay := RetryDelay(job.Attempts, result.RetryAfter, rand.Int64N)
					if err := w.Queue.Finish(ctx, job, result, delay); err != nil {
						w.report(err)
					}
				}
			}()
		}
	}
	for range w.Concurrency * 2 {
		<-done
	}
	return nil
}

func (w Worker) report(err error) {
	if w.OnError != nil {
		w.OnError(err)
	}
}
