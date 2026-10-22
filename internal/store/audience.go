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
	_, result, resultErr = s.GenerationInput(ctx, id, asOf)
	return
}

// GenerationInput reads campaign metadata and audience under one snapshot.
// The transaction ends before generation starts.
func (s Campaigns) GenerationInput(ctx context.Context, id int64, asOf time.Time) (c campaign.Campaign, result campaign.Preview, resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return c, result, ErrUnavailable
	}
	defer finishTx(tx, &resultErr)
	c, err = readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1`, id))
	if err != nil {
		return c, result, err
	}
	result, err = previewInTx(ctx, tx, c, asOf)
	if err != nil {
		return c, result, err
	}
	if err := tx.Commit(); err != nil {
		return c, result, databaseError(err)
	}
	return c, result, nil
}

func previewInTx(ctx context.Context, tx *sql.Tx, c campaign.Campaign, asOf time.Time) (result campaign.Preview, resultErr error) {
	result, _, resultErr = audienceInTx(ctx, tx, c, asOf, false)
	return
}

type audienceRecipient struct {
	id                                    int64
	channel, firstName, city, destination string
}

func audienceInTx(ctx context.Context, tx *sql.Tx, c campaign.Campaign, asOf time.Time, collect bool) (result campaign.Preview, recipients []audienceRecipient, resultErr error) {
	if !campaign.Editable(c.Status) {
		return result, recipients, campaign.ErrState
	}
	if len(c.Segment) == 0 {
		return result, recipients, campaign.ErrSegmentRequired
	}
	v, err := segment.ParseDocument(c.Segment)
	if err != nil {
		return result, recipients, ErrUnavailable
	}
	result = campaign.Preview{AsOf: asOf.UTC(), ByChannel: map[string]campaign.Bounds{}}
	unique := map[int64]bool{}
	for _, channel := range c.Channels {
		query, args, err := v.Query(channel, asOf)
		if err != nil {
			return result, recipients, ErrUnavailable
		}
		if collect {
			projection := "u.phone"
			if channel == "email" {
				projection = "u.email"
			}
			// Query is compiler-owned, parameterized SQL, never raw model output.
			query = "SELECT a.id,a.first_name,a.city," + projection + " FROM (" + query + ") a JOIN app.users u ON u.id=a.id ORDER BY a.id"
		}
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return result, recipients, ErrUnavailable
		}
		bounds := campaign.Bounds{}
		for rows.Next() {
			var id int64
			var firstName, city string
			var destination string
			var scanErr error
			if collect {
				scanErr = rows.Scan(&id, &firstName, &city, &destination)
			} else {
				scanErr = rows.Scan(&id, &firstName, &city)
			}
			if scanErr != nil {
				if closeErr := rows.Close(); closeErr != nil {
					return result, recipients, ErrUnavailable
				}
				return result, recipients, ErrUnavailable
			}
			if collect {
				recipients = append(recipients, audienceRecipient{id, channel, firstName, city, destination})
			}
			unique[id] = true
			bounds.Count++
			bounds.MaxFirstName = max(bounds.MaxFirstName, utf8.RuneCountInString(firstName))
			bounds.MaxCity = max(bounds.MaxCity, utf8.RuneCountInString(city))
		}
		rowErr := rows.Err()
		closeErr := rows.Close()
		if rowErr != nil || closeErr != nil {
			return result, recipients, ErrUnavailable
		}
		result.ByChannel[channel] = bounds
	}
	result.EligibleUnique = len(unique)
	return result, recipients, nil
}
