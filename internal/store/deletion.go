package store

import (
	"context"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

// Delete retains content and audit rows. The campaign lock serializes deletion
// with saves and approval. Existing restrictive foreign keys remain unchanged.
func (s Campaigns) Delete(ctx context.Context, id, expected int64, at time.Time) (resultErr error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return databaseError(err)
	}
	defer finishTx(tx, &resultErr)
	c, err := readCampaign(tx.QueryRowContext(ctx, `SELECT `+campaignColumns+` FROM app.campaigns WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if c.Revision != expected {
		return campaign.ErrRevision
	}
	if !campaign.Editable(c.Status) {
		return campaign.ErrState
	}
	var safe bool
	err = tx.QueryRowContext(ctx, `SELECT approved_at IS NULL AND approved_revision IS NULL
        AND approval_fingerprint IS NULL AND approval_audience_preview IS NULL
        AND NOT EXISTS (SELECT 1 FROM app.deliveries WHERE campaign_id=$1)
        FROM app.campaigns WHERE id=$1`, id).Scan(&safe)
	if err != nil {
		return databaseError(err)
	}
	if !safe {
		return campaign.ErrState
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app.campaigns SET deleted_at=$1,status='cancelled',revision=revision+1 WHERE id=$2`, at.UTC(), id); err != nil {
		return databaseError(err)
	}
	if err := tx.Commit(); err != nil {
		return databaseError(err)
	}
	return nil
}
