package store

import (
	"context"
	"database/sql"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

// Metrics reads only aggregates, never destinations, briefs or message payloads.
func (s Campaigns) Metrics(ctx context.Context) (result campaign.Metrics, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, ErrUnavailable
	}
	defer finishTx(tx, &resultErr)
	result.Deliveries = map[string]campaign.DeliveryCounts{"sms": {}, "email": {}}
	rows, err := tx.QueryContext(ctx, `SELECT channel,count(*),count(*) FILTER (WHERE status='pending'),count(*) FILTER (WHERE status='in_flight'),count(*) FILTER (WHERE status='sent'),count(*) FILTER (WHERE status='dead'),count(*) FILTER (WHERE status='skipped') FROM app.deliveries GROUP BY channel`)
	if err != nil {
		return result, ErrUnavailable
	}
	for rows.Next() {
		var channel string
		var c campaign.DeliveryCounts
		if err = rows.Scan(&channel, &c.Total, &c.Pending, &c.InFlight, &c.Sent, &c.Dead, &c.Skipped); err != nil {
			rows.Close()
			return result, ErrUnavailable
		}
		if channel != "sms" && channel != "email" {
			rows.Close()
			return result, ErrUnavailable
		}
		result.Deliveries[channel] = c
	}
	if rows.Err() != nil {
		rows.Close()
		return result, ErrUnavailable
	}
	if err = rows.Close(); err != nil {
		return result, ErrUnavailable
	}
	rows, err = tx.QueryContext(ctx, `SELECT call_name,outcome,count(*),sum(jsonb_array_length(validation_errors)) FROM app.llm_calls GROUP BY call_name,outcome ORDER BY call_name,outcome`)
	if err != nil {
		return result, ErrUnavailable
	}
	for rows.Next() {
		var m campaign.CallMetric
		if err = rows.Scan(&m.Call, &m.Outcome, &m.Count, &m.ValidationFailures); err != nil {
			rows.Close()
			return result, ErrUnavailable
		}
		result.Calls = append(result.Calls, m)
	}
	if rows.Err() != nil {
		rows.Close()
		return result, ErrUnavailable
	}
	if err = rows.Close(); err != nil {
		return result, ErrUnavailable
	}
	if err = tx.Commit(); err != nil {
		return result, ErrUnavailable
	}
	return result, nil
}
