package store

import (
	"context"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

// List uses the primary key as a stable cursor; a concurrent creation cannot
// shift later pages. The extra row only establishes whether another page exists.
func (s Campaigns) List(ctx context.Context, before int64, limit int) (campaign.Page, error) {
	page := campaign.Page{Campaigns: []campaign.Campaign{}}
	if before < 0 || limit < 1 || limit > 100 {
		return page, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	rows, err := s.DB.QueryContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE deleted_at IS NULL AND ($1::bigint=0 OR id<$1) ORDER BY id DESC LIMIT $2`, before, limit+1)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		c, err := readCampaign(rows)
		if err != nil {
			return campaign.Page{}, err
		}
		if len(page.Campaigns) == limit {
			last := page.Campaigns[limit-1].ID
			page.NextBefore = &last
			break
		}
		page.Campaigns = append(page.Campaigns, c)
	}
	if rows.Err() != nil {
		return campaign.Page{}, ErrUnavailable
	}
	return page, nil
}
