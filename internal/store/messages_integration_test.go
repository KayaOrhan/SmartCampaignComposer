//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func messageRepo(t *testing.T) (Campaigns, context.Context, campaign.Campaign, time.Time) {
	t.Helper()
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	repo := Campaigns{DB: db, Timeout: 5 * time.Second}
	c, err := repo.Create(ctx, campaign.Draft{Brief: "message fixture", Channels: []string{"sms", "email"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"language","op":"in","value":["tr"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err = repo.PutSegment(ctx, c.ID, c.Revision, v)
	if err != nil {
		t.Fatal(err)
	}
	return repo, ctx, c, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
}

func messageBody(sms string) []byte {
	b, _ := json.Marshal(campaign.Messages{SMS: &campaign.SMS{Body: sms}, Email: &campaign.Email{Subject: "Offer", Body: "Email body"}})
	return b
}

func TestSavedMessagesLifecycle(t *testing.T) {
	repo, ctx, c, at := messageRepo(t)
	empty, err := repo.GetMessages(ctx, c.ID)
	if err != nil || empty.Messages != nil || empty.Validation != nil || empty.Status != "draft" {
		t.Fatal("initial messages", err)
	}
	first, err := repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Hello {{first_name}}"), at)
	if err != nil || first.Status != "content_ready" || first.Revision != c.Revision+1 || first.Validation == nil || first.Validation.Revision != first.Revision {
		t.Fatal("save failed", err)
	}
	stored, err := repo.GetMessages(ctx, c.ID)
	if err != nil || !bytes.Equal(stored.Messages, first.Messages) || !stored.Validation.Audience.AsOf.Equal(at) {
		t.Fatal("round trip failed", err)
	}
	if _, err := repo.Preview(ctx, c.ID, at); err != nil {
		t.Fatal("content ready preview", err)
	}
	input, _, err := repo.GenerationInput(ctx, c.ID, at)
	if err != nil || input.Revision != first.Revision {
		t.Fatal("generation input", err)
	}
	for _, raw := range [][]byte{
		messageBody(strings.Repeat("x", 161)),
		messageBody("{{unknown}}"),
		[]byte(`{"sms":{"body":"ok","body":"other"},"email":{"subject":"a","body":"b"}}`),
		[]byte(`{"sms":{"body":"ok"}}`),
		[]byte(`{"sms":{"body":"ok","sql":"DROP"},"email":{"subject":"a","body":"b"}}`),
	} {
		if _, err := repo.PutMessages(ctx, c.ID, first.Revision, raw, at); err == nil {
			t.Fatal("invalid edit accepted")
		}
	}
	if _, err := repo.PutMessages(ctx, c.ID, c.Revision, messageBody("stale"), at); !errors.Is(err, campaign.ErrRevision) {
		t.Fatal("stale generation accepted", err)
	}
	unchanged, err := repo.GetMessages(ctx, c.ID)
	if err != nil || unchanged.Revision != first.Revision || !bytes.Equal(unchanged.Messages, first.Messages) {
		t.Fatal("rejection changed messages", err)
	}
	edited, err := repo.PutMessages(ctx, c.ID, first.Revision, messageBody("Edited"), at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	v, err := segment.ParseDocument(c.Segment)
	if err != nil {
		t.Fatal(err)
	}
	c, err = repo.PutSegment(ctx, c.ID, edited.Revision, v)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := repo.GetMessages(ctx, c.ID)
	if err != nil || stale.Status != "draft" || stale.Validation != nil || !bytes.Equal(stale.Messages, edited.Messages) || stale.Revision != c.Revision {
		t.Fatal("segment change destroyed text or retained readiness", err)
	}
	revalidated, err := repo.PutMessages(ctx, c.ID, c.Revision, stale.Messages, at)
	if err != nil || revalidated.Status != "content_ready" {
		t.Fatal("revalidation failed", err)
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET status='approved' WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutMessages(ctx, c.ID, revalidated.Revision, messageBody("forbidden"), at); !errors.Is(err, campaign.ErrState) {
		t.Fatal("approved messages editable", err)
	}
}

func TestMessageSaveUsesCurrentAudienceBounds(t *testing.T) {
	repo, ctx, c, at := messageRepo(t)
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) VALUES (1,'A','Example','fixture@example.test','mock-phone','X','x','tr',$1)`, at); err != nil {
		t.Fatal("audience fixture failed")
	}
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.consents(user_id,channel,granted,updated_at) VALUES (1,'sms',true,$1),(1,'email',true,$1)`, at); err != nil {
		t.Fatal("consent fixture failed")
	}
	body := messageBody(strings.Repeat("x", 159) + "{{first_name}}")
	preview, err := repo.Preview(ctx, c.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := campaign.ValidateMessages(body, c.Channels, preview.ByChannel); err != nil {
		t.Fatal("fixture must fit original preview", err)
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.users SET first_name='Longer' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	_, err = repo.PutMessages(ctx, c.ID, c.Revision, body, at)
	var v *segment.ValidationError
	if !errors.As(err, &v) || v.Code != "message_too_long" {
		t.Fatal("old bounds trusted", err)
	}
	stored, err := repo.GetMessages(ctx, c.ID)
	if err != nil || stored.Messages != nil || stored.Revision != c.Revision {
		t.Fatal("failed validation changed campaign", err)
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.consents SET granted=false WHERE user_id=1 AND channel='sms'`); err != nil {
		t.Fatal(err)
	}
	// An empty channel must use schema bounds, never a zero-length name.
	if _, err := repo.PutMessages(ctx, c.ID, c.Revision, body, at); err == nil {
		t.Fatal("empty channel bypassed bounds")
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.users SET first_name='A' WHERE id=1; UPDATE app.consents SET granted=true WHERE user_id=1 AND channel='sms'`); err != nil {
		t.Fatal(err)
	}
	// A valid rendered template can be much longer than 160 raw characters.
	longTemplate := messageBody(strings.Repeat("{{first_name}}", 160))
	saved, err := repo.PutMessages(ctx, c.ID, c.Revision, longTemplate, at)
	if err != nil {
		t.Fatal("raw placeholder lengths incorrectly constrained", err)
	}
	stored, err = repo.GetMessages(ctx, c.ID)
	if err != nil || !bytes.Equal(saved.Messages, stored.Messages) {
		t.Fatal("long placeholder round trip", err)
	}
}

func TestSingleChannelMessagePersistenceAndCancellation(t *testing.T) {
	repo, ctx, _, at := messageRepo(t)
	for _, ch := range []string{"sms", "email"} {
		c, err := repo.Create(ctx, campaign.Draft{Brief: "single channel", Channels: []string{ch}})
		if err != nil {
			t.Fatal(err)
		}
		v, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"language","op":"in","value":["tr"]}]}`))
		if err != nil {
			t.Fatal(err)
		}
		c, err = repo.PutSegment(ctx, c.ID, c.Revision, v)
		if err != nil {
			t.Fatal(err)
		}
		body := []byte(`{"sms":{"body":"Hi"}}`)
		if ch == "email" {
			body = []byte(`{"email":{"subject":"Hi","body":"Body"}}`)
		}
		saved, err := repo.PutMessages(ctx, c.ID, c.Revision, body, at)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := repo.GetMessages(ctx, c.ID)
		if err != nil || !bytes.Equal(saved.Messages, stored.Messages) {
			t.Fatal("single channel round trip", err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := repo.PutMessages(cancelled, c.ID, saved.Revision, body, at); err == nil {
			t.Fatal("cancelled save succeeded")
		}
		unchanged, err := repo.GetMessages(ctx, c.ID)
		if err != nil || unchanged.Revision != saved.Revision {
			t.Fatal("cancelled save changed revision", err)
		}
	}
}

func TestMessageReplacementRollsBack(t *testing.T) {
	repo, ctx, c, at := messageRepo(t)
	first, err := repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Original"), at)
	if err != nil {
		t.Fatal(err)
	}
	// A test-only database constraint fails the email insert after SMS was replaced.
	if _, err := repo.DB.ExecContext(ctx, `ALTER TABLE app.campaign_messages ADD CONSTRAINT reject_test_subject CHECK (subject IS DISTINCT FROM 'Reject')`); err != nil {
		t.Fatal(err)
	}
	_, err = repo.PutMessages(ctx, c.ID, first.Revision, []byte(`{"sms":{"body":"Replacement"},"email":{"subject":"Reject","body":"Body"}}`), at)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("database failure not surfaced", err)
	}
	stored, err := repo.GetMessages(ctx, c.ID)
	if err != nil || stored.Revision != first.Revision || !bytes.Equal(stored.Messages, first.Messages) {
		t.Fatal("partial set committed", err)
	}
}

func TestMessageConcurrentWrites(t *testing.T) {
	for _, againstSegment := range []bool{false, true} {
		t.Run(map[bool]string{false: "edit versus edit", true: "edit versus segment"}[againstSegment], func(t *testing.T) {
			repo, ctx, c, at := messageRepo(t)
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := repo.PutMessages(ctx, c.ID, c.Revision, messageBody("First"), at)
				results <- err
			}()
			go func() {
				<-start
				var err error
				if againstSegment {
					v, e := segment.ParseDocument(c.Segment)
					if e != nil {
						results <- e
						return
					}
					_, err = repo.PutSegment(ctx, c.ID, c.Revision, v)
				} else {
					_, err = repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Second"), at)
				}
				results <- err
			}()
			close(start)
			wins, conflicts := 0, 0
			for range 2 {
				err := <-results
				if err == nil {
					wins++
				} else if errors.Is(err, campaign.ErrRevision) || errors.Is(err, campaign.ErrConcurrentUpdate) {
					conflicts++
				} else {
					t.Fatal(err)
				}
			}
			stored, err := repo.GetMessages(ctx, c.ID)
			if err != nil || wins != 1 || conflicts != 1 || stored.Revision != c.Revision+1 {
				t.Fatal("concurrent update lost", err)
			}
			if stored.Status == "content_ready" && (stored.Validation == nil || stored.Messages == nil) {
				t.Fatal("partial ready content")
			}
		})
	}
}
