package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

func (s Campaigns) GetMessages(ctx context.Context, id int64) (result campaign.SavedMessages, resultErr error) {
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
	result = campaign.SavedMessages{CampaignID: id, Revision: c.Revision, Status: c.Status}
	var revision sql.NullInt64
	var preview []byte
	if err := tx.QueryRowContext(ctx, `SELECT messages_validated_revision,message_validation_preview FROM app.campaigns WHERE id=$1`, id).Scan(&revision, &preview); err != nil {
		return result, databaseError(err)
	}
	if revision.Valid {
		var p campaign.Preview
		if err := json.Unmarshal(preview, &p); err != nil {
			return result, ErrUnavailable
		}
		result.Validation = &campaign.MessageValidation{Revision: revision.Int64, Audience: p}
	}
	rows, err := tx.QueryContext(ctx, `SELECT channel,subject,body FROM app.campaign_messages WHERE campaign_id=$1 ORDER BY channel`, id)
	if err != nil {
		return result, databaseError(err)
	}
	m := campaign.Messages{}
	count := 0
	for rows.Next() {
		var channel, body string
		var subject sql.NullString
		if err := rows.Scan(&channel, &subject, &body); err != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return result, ErrUnavailable
			}
			return result, databaseError(err)
		}
		count++
		switch channel {
		case "sms":
			m.SMS = &campaign.SMS{Body: body}
		case "email":
			m.Email = &campaign.Email{Subject: subject.String, Body: body}
		}
	}
	rowErr, closeErr := rows.Err(), rows.Close()
	if rowErr != nil || closeErr != nil {
		return result, ErrUnavailable
	}
	if count > 0 {
		body, err := json.Marshal(m)
		if err != nil {
			return result, ErrUnavailable
		}
		// Stale retained text still needs structural validation; minimal possible
		// profile lengths avoid claiming validity against a current audience.
		bounds := map[string]campaign.Bounds{}
		for _, ch := range c.Channels {
			bounds[ch] = campaign.Bounds{Count: 1, MaxFirstName: 1, MaxCity: 1}
		}
		if result.Validation != nil {
			bounds = result.Validation.Audience.ByChannel
		}
		v, err := campaign.ValidateMessages(body, c.Channels, bounds)
		if err != nil {
			return result, ErrUnavailable
		}
		result.Messages = v.JSON()
	} else if result.Validation != nil {
		return result, ErrUnavailable
	}
	if err := tx.Commit(); err != nil {
		return result, databaseError(err)
	}
	return result, nil
}

func (s Campaigns) PutMessages(ctx context.Context, id, expected int64, body []byte, asOf time.Time) (result campaign.SavedMessages, resultErr error) {
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
	audience, err := previewInTx(ctx, tx, c, asOf)
	if err != nil {
		return result, err
	}
	v, err := campaign.ValidateMessages(body, c.Channels, audience.ByChannel)
	if err != nil {
		return result, err
	}
	var m campaign.Messages
	if err := json.Unmarshal(v.JSON(), &m); err != nil {
		return result, ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM app.campaign_messages WHERE campaign_id=$1`, id); err != nil {
		return result, databaseError(err)
	}
	if m.SMS != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app.campaign_messages(campaign_id,channel,body) VALUES ($1,'sms',$2)`, id, m.SMS.Body); err != nil {
			return result, databaseError(err)
		}
	}
	if m.Email != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app.campaign_messages(campaign_id,channel,subject,body) VALUES ($1,'email',$2,$3)`, id, m.Email.Subject, m.Email.Body); err != nil {
			return result, databaseError(err)
		}
	}
	preview, err := json.Marshal(audience)
	if err != nil {
		return result, ErrUnavailable
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app.campaigns SET revision=revision+1,status='content_ready',messages_validated_revision=revision+1,message_validation_preview=$1::jsonb WHERE id=$2`, string(preview), id); err != nil {
		return result, databaseError(err)
	}
	if err := tx.Commit(); err != nil {
		return result, databaseError(err)
	}
	return campaign.SavedMessages{CampaignID: id, Revision: c.Revision + 1, Status: "content_ready", Messages: v.JSON(), Validation: &campaign.MessageValidation{Revision: c.Revision + 1, Audience: audience}}, nil
}
