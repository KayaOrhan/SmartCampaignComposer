package store

import (
	"context"
	"database/sql"
	"time"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (s Campaigns) Preview(ctx context.Context, id int64, asOf time.Time) (result campaign.Preview, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return result, ErrUnavailable
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1`, id))
	if err != nil {
		return result, err
	}
	if c.Status != "draft" {
		return result, campaign.ErrState
	}
	if len(c.Segment) == 0 {
		return result, campaign.ErrSegmentRequired
	}
	v, err := segment.ParseDocument(c.Segment)
	if err != nil {
		return result, ErrUnavailable
	}
	result = campaign.Preview{AsOf: asOf.UTC(), ByChannel: map[string]campaign.Bounds{}}
	unique := map[int64]bool{}
	for _, channel := range c.Channels {
		query, args, err := v.Query(channel, asOf)
		if err != nil {
			return result, ErrUnavailable
		}
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return result, ErrUnavailable
		}
		bounds := campaign.Bounds{}
		for rows.Next() {
			var id int64
			var firstName, city string
			if err := rows.Scan(&id, &firstName, &city); err != nil {
				if closeErr := rows.Close(); closeErr != nil {
					return result, ErrUnavailable
				}
				return result, ErrUnavailable
			}
			unique[id] = true
			bounds.Count++
			bounds.MaxFirstName = max(bounds.MaxFirstName, utf8.RuneCountInString(firstName))
			bounds.MaxCity = max(bounds.MaxCity, utf8.RuneCountInString(city))
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if rowErr != nil || closeErr != nil {
			return result, ErrUnavailable
		}
		result.ByChannel[channel] = bounds
	}
	result.EligibleUnique = len(unique)
	if err := tx.Commit(); err != nil {
		return result, ErrUnavailable
	}
	return result, nil
}
