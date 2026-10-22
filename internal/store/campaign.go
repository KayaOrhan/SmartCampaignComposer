package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

var ErrUnavailable = errors.New("campaign database operation failed")

type Campaigns struct {
	DB      *sql.DB
	Timeout time.Duration
}

func (s Campaigns) Create(ctx context.Context, draft campaign.Draft) (campaign.Campaign, error) {
	draft, err := draft.Validate()
	if err != nil {
		return campaign.Campaign{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	var c campaign.Campaign
	err = s.DB.QueryRowContext(ctx, `INSERT INTO app.campaigns(brief,channels) VALUES ($1,$2) RETURNING id,brief,channels,status,created_at`, draft.Brief, draft.Channels).Scan(&c.ID, &c.Brief, &c.Channels, &c.Status, &c.CreatedAt)
	if err != nil {
		return campaign.Campaign{}, ErrUnavailable
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return c, nil
}

type rowScanner interface{ Scan(...any) error }

func readCampaign(row rowScanner) (campaign.Campaign, error) {
	var c campaign.Campaign
	var raw []byte
	err := row.Scan(&c.ID, &c.Brief, &c.Channels, &raw, &c.Status, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, campaign.ErrNotFound
	}
	if err != nil {
		return c, ErrUnavailable
	}
	if raw != nil {
		validated, err := segment.ParseDocument(raw)
		if err != nil {
			return campaign.Campaign{}, ErrUnavailable
		}
		c.Segment = validated.JSON()
	}
	c.CreatedAt = c.CreatedAt.UTC()
	return c, nil
}

const campaignColumns = `id,brief,channels,segment_json,status,created_at`

func (s Campaigns) Get(ctx context.Context, id int64) (campaign.Campaign, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	return readCampaign(s.DB.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1`, id))
}

func (s Campaigns) PutSegment(ctx context.Context, id int64, v segment.Validated) (result campaign.Campaign, resultErr error) {
	if _, err := segment.ParseDocument(v.JSON()); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, ErrUnavailable
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return result, err
	}
	if c.Status != "draft" {
		return result, campaign.ErrState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app.campaigns SET segment_json=$1::jsonb WHERE id=$2`, string(v.JSON()), id); err != nil {
		return result, ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		return result, ErrUnavailable
	}
	c.Segment = v.JSON()
	return c, nil
}

func finishTx(tx *sql.Tx, result *error) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		*result = errors.Join(*result, ErrUnavailable)
	}
}
