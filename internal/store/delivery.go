package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
)

type Deliveries struct {
	DB      *sql.DB
	Timeout time.Duration
}

func (s Deliveries) Claim(ctx context.Context, channel string, lease time.Duration) (delivery.Job, error) {
	var j delivery.Job
	if (channel != "sms" && channel != "email") || lease <= 0 {
		return j, delivery.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	err := s.DB.QueryRowContext(ctx, `WITH candidate AS (
        SELECT d.id FROM app.deliveries d JOIN app.campaigns c ON c.id=d.campaign_id
        WHERE d.channel=$1 AND d.status='pending' AND d.next_attempt_at<=clock_timestamp()
          AND d.attempts<5 AND c.status='sending'
        ORDER BY d.next_attempt_at,d.id FOR UPDATE OF d SKIP LOCKED LIMIT 1)
        UPDATE app.deliveries d SET status='in_flight',attempts=attempts+1,
          claim_generation=claim_generation+1,locked_at=clock_timestamp(),
          lease_expires_at=clock_timestamp()+$2::bigint*interval '1 microsecond',updated_at=clock_timestamp()
        FROM candidate WHERE d.id=candidate.id
        RETURNING d.id,d.campaign_id,d.user_id,d.channel,d.destination,d.subject,d.body,d.idempotency_key,d.attempts,d.claim_generation`, channel, lease.Microseconds()).Scan(&j.ID, &j.CampaignID, &j.UserID, &j.Payload.Channel, &j.Payload.Destination, &j.Payload.Subject, &j.Payload.Body, &j.Key, &j.Attempts, &j.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Job{}, nil
	}
	if err != nil {
		return delivery.Job{}, delivery.ErrUnavailable
	}
	return j, nil
}

func (s Deliveries) Recover(ctx context.Context) (result error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return delivery.ErrUnavailable
	}
	defer finishTx(tx, &result)
	// Match Finish's campaign-before-delivery lock order. A second reaper skips
	// a busy campaign instead of deadlocking with a result transaction.
	var campaignID int64
	err = tx.QueryRowContext(ctx, `SELECT c.id FROM app.campaigns c WHERE EXISTS(
        SELECT 1 FROM app.deliveries d WHERE d.campaign_id=c.id AND d.status='in_flight' AND d.lease_expires_at<=clock_timestamp())
        ORDER BY c.id FOR UPDATE OF c SKIP LOCKED LIMIT 1`).Scan(&campaignID)
	if errors.Is(err, sql.ErrNoRows) {
		if tx.Commit() != nil {
			return delivery.ErrUnavailable
		}
		return nil
	}
	if err != nil {
		return delivery.ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, `WITH expired AS (
        SELECT id FROM app.deliveries WHERE campaign_id=$1 AND status='in_flight' AND lease_expires_at<=clock_timestamp()
        ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 100)
        UPDATE app.deliveries d SET status=CASE WHEN attempts>=5 THEN 'dead' ELSE 'pending' END,
          claim_generation=claim_generation+1,locked_at=NULL,lease_expires_at=NULL,
          next_attempt_at=clock_timestamp(),last_error_code='lease_expired',updated_at=clock_timestamp()
        FROM expired WHERE d.id=expired.id`, campaignID); err != nil {
		return delivery.ErrUnavailable
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.campaigns c SET status='completed' WHERE id=$1 AND status='sending'
        AND NOT EXISTS(SELECT 1 FROM app.deliveries d WHERE d.campaign_id=c.id AND d.status IN ('pending','in_flight'))`, campaignID); err != nil {
		return delivery.ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return delivery.ErrUnavailable
	}
	return nil
}

func (s Deliveries) Finish(ctx context.Context, j delivery.Job, o delivery.Outcome, delay time.Duration) (result error) {
	if o.Status == "pending" && j.Attempts >= delivery.MaxAttempts {
		o.Status = "dead"
	}
	if o.Status != "sent" && o.Status != "dead" && o.Status != "pending" && o.Status != "skipped" {
		return delivery.ErrUnavailable
	}
	if len(o.Code) > 64 || len(o.MessageID) > 128 || delay < 0 {
		return delivery.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return delivery.ErrUnavailable
	}
	defer finishTx(tx, &result)
	var campaignID int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM app.campaigns WHERE id=$1 FOR UPDATE`, j.CampaignID).Scan(&campaignID); err != nil {
		return delivery.ErrUnavailable
	}
	r, err := tx.ExecContext(ctx, `UPDATE app.deliveries SET status=$1,last_error_code=NULLIF($2,''),provider_message_id=NULLIF($3,''),
        next_attempt_at=clock_timestamp()+$4::bigint*interval '1 microsecond',locked_at=NULL,lease_expires_at=NULL,updated_at=clock_timestamp()
        WHERE id=$5 AND campaign_id=$6 AND status='in_flight' AND claim_generation=$7 AND lease_expires_at>clock_timestamp()`, o.Status, o.Code, o.MessageID, delay.Microseconds(), j.ID, j.CampaignID, j.Generation)
	if err != nil {
		return delivery.ErrUnavailable
	}
	n, err := r.RowsAffected()
	if err != nil {
		return delivery.ErrUnavailable
	}
	if n != 1 {
		return delivery.ErrLostClaim
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app.campaigns c SET status='completed' WHERE id=$1 AND status='sending'
        AND NOT EXISTS(SELECT 1 FROM app.deliveries d WHERE d.campaign_id=c.id AND d.status IN ('pending','in_flight'))`, j.CampaignID); err != nil {
		return delivery.ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return delivery.ErrUnavailable
	}
	return nil
}

// A session advisory lock serializes the channel across processes, without a
// transaction spanning HTTP. Keeping it through the response is conservative:
// slow mock calls reduce throughput rather than accumulating burst credit.
func (s Deliveries) Dispatch(ctx context.Context, j delivery.Job, lease time.Duration, send func(context.Context, delivery.Job) delivery.Outcome) (out delivery.Outcome, result error) {
	connCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	conn, err := s.DB.Conn(connCtx)
	if err != nil {
		return out, delivery.ErrUnavailable
	}
	defer func() {
		if err := conn.Close(); err != nil {
			result = errors.Join(result, delivery.ErrUnavailable)
		}
	}()
	key := int64(730001)
	if j.Payload.Channel == "email" {
		key++
	} else if j.Payload.Channel != "sms" {
		return out, delivery.ErrUnavailable
	}
	locked := false
	renewAt := time.Now().Add(lease / 3)
	renew := func() error {
		if time.Now().Before(renewAt) {
			return nil
		}
		q, stop := context.WithTimeout(ctx, s.Timeout)
		defer stop()
		var id int64
		err := conn.QueryRowContext(q, `UPDATE app.deliveries SET lease_expires_at=clock_timestamp()+$1::bigint*interval '1 microsecond'
            WHERE id=$2 AND status='in_flight' AND claim_generation=$3 AND lease_expires_at>clock_timestamp() RETURNING id`, lease.Microseconds(), j.ID, j.Generation).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return delivery.ErrLostClaim
		}
		if err != nil {
			return delivery.ErrUnavailable
		}
		renewAt = time.Now().Add(lease / 3)
		return nil
	}
	defer func() {
		if !locked {
			return
		}
		cleanup, stop := context.WithTimeout(context.Background(), s.Timeout)
		defer stop()
		// Persist spacing even on cancellation; failure discards the connection
		// so no session lock can leak back into the pool.
		_, e1 := conn.ExecContext(cleanup, `UPDATE app.delivery_channels SET next_start_at=clock_timestamp()+interval '10 milliseconds' WHERE channel=$1`, j.Payload.Channel)
		var unlocked bool
		e2 := conn.QueryRowContext(cleanup, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked)
		if e1 != nil || e2 != nil || !unlocked {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			result = errors.Join(result, delivery.ErrUnavailable)
		}
	}()
	for !locked {
		if err = renew(); err != nil {
			return out, err
		}
		q, stop := context.WithTimeout(ctx, s.Timeout)
		err = conn.QueryRowContext(q, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked)
		stop()
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return out, delivery.ErrUnavailable
		}
		if !locked {
			if err = delivery.Wait(ctx, 10*time.Millisecond); err != nil {
				return out, err
			}
		}
	}
	var waitSeconds float64
	q, stop := context.WithTimeout(ctx, s.Timeout)
	err = conn.QueryRowContext(q, `SELECT greatest(0,extract(epoch FROM next_start_at-clock_timestamp()))::float8 FROM app.delivery_channels WHERE channel=$1`, j.Payload.Channel).Scan(&waitSeconds)
	stop()
	if err != nil {
		return out, delivery.ErrUnavailable
	}
	waitUntil := time.Now().Add(time.Duration(waitSeconds * float64(time.Second)))
	for time.Now().Before(waitUntil) {
		if err = renew(); err != nil {
			return out, err
		}
		if err = delivery.Wait(ctx, min(10*time.Millisecond, time.Until(waitUntil))); err != nil {
			return out, err
		}
	}
	q, stop = context.WithTimeout(ctx, s.Timeout)
	var allowed bool
	var expires time.Time
	// This is only a final consent/ownership check, not audience recomputation.
	err = conn.QueryRowContext(q, `WITH gate AS (
        UPDATE app.delivery_channels SET next_start_at=clock_timestamp()+$1::bigint*interval '1 microsecond' WHERE channel=$4 RETURNING channel)
        UPDATE app.deliveries d SET lease_expires_at=clock_timestamp()+$1::bigint*interval '1 microsecond'
        FROM app.campaigns c,gate WHERE d.id=$2 AND d.claim_generation=$3 AND d.status='in_flight' AND d.channel=gate.channel
          AND d.lease_expires_at>clock_timestamp() AND c.id=d.campaign_id
        RETURNING c.status='sending' AND EXISTS(SELECT 1 FROM app.consents s WHERE s.user_id=d.user_id AND s.channel=d.channel AND s.granted),d.lease_expires_at`, lease.Microseconds(), j.ID, j.Generation, j.Payload.Channel).Scan(&allowed, &expires)
	stop()
	if errors.Is(err, sql.ErrNoRows) {
		return out, delivery.ErrLostClaim
	}
	if err != nil {
		return out, delivery.ErrUnavailable
	}
	if !allowed {
		return delivery.Outcome{Status: "skipped", Code: "consent_or_campaign_revoked"}, nil
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	// The durable reservation survives a hard process kill. Normal cleanup
	// shortens it to response completion + 10ms. An old paused worker cannot
	// start HTTP after its lease has elapsed.
	callCtx, callCancel := context.WithDeadline(ctx, expires)
	defer callCancel()
	return send(callCtx, j), nil
}
