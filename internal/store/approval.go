package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

// Frozen recipient data is private to storage. Never return it through HTTP or
// attach it to errors/logs; approval/status responses contain aggregates only.
type frozenDelivery struct {
	UserID         int64   `json:"user_id"`
	Channel        string  `json:"channel"`
	Destination    string  `json:"destination"`
	Subject        *string `json:"subject"`
	Body           string  `json:"body"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func approvalSnapshot(ctx context.Context, tx *sql.Tx, c campaign.Campaign, asOf time.Time) (campaign.ApprovalPreview, []frozenDelivery, error) {
	result := campaign.ApprovalPreview{Revision: c.Revision}
	if c.Status != "content_ready" {
		return result, nil, campaign.ErrState
	}
	p, recipients, err := audienceInTx(ctx, tx, c, asOf, true)
	if err != nil {
		return result, nil, err
	}
	result.Preview = p
	saved, err := messagesInTx(ctx, tx, c)
	if err != nil {
		return result, nil, err
	}
	if saved.Validation == nil || saved.Validation.Revision != c.Revision || len(saved.Messages) == 0 {
		return result, nil, campaign.ErrState
	}
	if p.EligibleUnique == 0 {
		return result, nil, nil
	}
	v, err := campaign.ValidateMessages(saved.Messages, c.Channels, p.ByChannel)
	if err != nil {
		return result, nil, err
	}
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	// Versioned, unambiguous canonical input. The timestamp is deliberately
	// excluded: only revision, membership, destinations and content matter.
	if err := encoder.Encode(struct {
		Version              int
		CampaignID, Revision int64
		Channels             []string
		Messages             json.RawMessage
	}{1, c.ID, c.Revision, c.Channels, v.JSON()}); err != nil {
		return result, nil, ErrUnavailable
	}
	frozen := make([]frozenDelivery, 0, len(recipients))
	for _, r := range recipients {
		if err := ctx.Err(); err != nil {
			return result, nil, ErrUnavailable
		}
		m, err := v.Render(r.channel, r.firstName, r.city)
		if err != nil {
			return result, nil, err
		}
		d := frozenDelivery{UserID: r.id, Channel: r.channel, Destination: r.destination, Subject: m.Subject, Body: m.Body, IdempotencyKey: fmt.Sprintf("campaign:%d:user:%d:%s", c.ID, r.id, r.channel)}
		if err := encoder.Encode(d); err != nil {
			return result, nil, ErrUnavailable
		}
		frozen = append(frozen, d)
	}
	fingerprint := hex.EncodeToString(hash.Sum(nil))
	result.ApprovalFingerprint = &fingerprint
	return result, frozen, nil
}

func (s Campaigns) ApprovalPreview(ctx context.Context, id int64, asOf time.Time) (result campaign.ApprovalPreview, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, databaseError(err)
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1`, id))
	if err != nil {
		return result, err
	}
	if c.Status == "content_ready" {
		result, _, err = approvalSnapshot(ctx, tx, c, asOf)
	} else {
		result.Revision = c.Revision
		result.Preview, err = previewInTx(ctx, tx, c, asOf)
	}
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, databaseError(err)
	}
	return result, nil
}

func (s Campaigns) Approve(ctx context.Context, id, expected int64, fingerprint string, asOf time.Time) (result campaign.DeliveryStatus, resultErr error) {
	if err := campaign.ValidateApprovalFingerprint(fingerprint); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return result, databaseError(err)
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return result, err
	}
	if c.Revision != expected {
		return result, campaign.ErrRevision
	}
	if c.Status == "sending" || c.Status == "completed" {
		var existing sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT approval_fingerprint FROM app.campaigns WHERE id=$1`, id).Scan(&existing); err != nil {
			return result, databaseError(err)
		}
		if !existing.Valid || existing.String != fingerprint {
			return result, campaign.ErrState
		}
	} else {
		preview, frozen, err := approvalSnapshot(ctx, tx, c, asOf)
		if err != nil {
			return result, err
		}
		if preview.EligibleUnique == 0 {
			return result, campaign.ErrEmptyAudience
		}
		if preview.ApprovalFingerprint == nil || *preview.ApprovalFingerprint != fingerprint {
			return result, &campaign.ApprovalPreviewChanged{Preview: preview}
		}
		// Fixed-size chunks avoid a database round trip per recipient while
		// keeping each bound JSON parameter small. No SQL comes from model text.
		for start := 0; start < len(frozen); start += 200 {
			batch := frozen[start:min(start+200, len(frozen))]
			encoded, err := json.Marshal(batch)
			if err != nil {
				return result, ErrUnavailable
			}
			inserted, err := tx.ExecContext(ctx, `INSERT INTO app.deliveries(campaign_id,user_id,channel,destination,subject,body,idempotency_key)
                SELECT $1,r.user_id,r.channel,r.destination,r.subject,r.body,r.idempotency_key
                FROM jsonb_to_recordset($2::jsonb) AS r(user_id bigint,channel text,destination text,subject text,body text,idempotency_key text)
                ON CONFLICT (campaign_id,user_id,channel) DO NOTHING`, id, string(encoded))
			if err != nil {
				return result, databaseError(err)
			}
			count, err := inserted.RowsAffected()
			if err != nil || count != int64(len(batch)) {
				return result, ErrUnavailable
			}
		}
		encoded, err := json.Marshal(preview.Preview)
		if err != nil {
			return result, ErrUnavailable
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app.campaigns SET status='sending',revision=revision+1,approved_at=$1,approved_revision=revision,approval_fingerprint=$2,approval_audience_preview=$3::jsonb WHERE id=$4`, asOf.UTC(), fingerprint, string(encoded), id); err != nil {
			return result, databaseError(err)
		}
		c.Status = "sending"
		c.Revision++
	}
	result, err = deliveryStatusInTx(ctx, tx, c)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, databaseError(err)
	}
	return result, nil
}

func (s Campaigns) GetDeliveryStatus(ctx context.Context, id int64) (result campaign.DeliveryStatus, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, databaseError(err)
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1`, id))
	if err != nil {
		return result, err
	}
	result, err = deliveryStatusInTx(ctx, tx, c)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, databaseError(err)
	}
	return result, nil
}

func deliveryStatusInTx(ctx context.Context, tx *sql.Tx, c campaign.Campaign) (result campaign.DeliveryStatus, resultErr error) {
	result = campaign.DeliveryStatus{CampaignID: c.ID, Revision: c.Revision, Status: c.Status, ByChannel: map[string]campaign.DeliveryCounts{}}
	var at sql.NullTime
	var source sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT approved_at,approved_revision FROM app.campaigns WHERE id=$1`, c.ID).Scan(&at, &source); err != nil {
		return result, databaseError(err)
	}
	if at.Valid {
		utc := at.Time.UTC()
		result.ApprovedAt = &utc
	}
	if source.Valid {
		result.ApprovedRevision = &source.Int64
	}
	for _, ch := range c.Channels {
		result.ByChannel[ch] = campaign.DeliveryCounts{}
	}
	rows, err := tx.QueryContext(ctx, `SELECT channel,status,count(*) FROM app.deliveries WHERE campaign_id=$1 GROUP BY channel,status`, c.ID)
	if err != nil {
		return result, databaseError(err)
	}
	for rows.Next() {
		var channel, status string
		var count int64
		if err := rows.Scan(&channel, &status, &count); err != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return result, ErrUnavailable
			}
			return result, databaseError(err)
		}
		channelCounts := result.ByChannel[channel]
		for _, counts := range []*campaign.DeliveryCounts{&result.Counts, &channelCounts} {
			counts.Total += count
			switch status {
			case "pending":
				counts.Pending += count
			case "in_flight":
				counts.InFlight += count
			case "sent":
				counts.Sent += count
			case "dead":
				counts.Dead += count
			case "skipped":
				counts.Skipped += count
			}
		}
		result.ByChannel[channel] = channelCounts
	}
	rowErr, closeErr := rows.Err(), rows.Close()
	if rowErr != nil || closeErr != nil {
		return result, ErrUnavailable
	}
	return result, nil
}
