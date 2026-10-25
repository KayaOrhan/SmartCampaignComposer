//go:build integration

package store

import (
	"errors"
	"testing"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func TestDeleteRetainsAuditAndHidesCampaign(t *testing.T) {
	repo, ctx, c, at := approvalFixture(t)
	call := llm.Call{CampaignID: c.ID, Name: "ExtractSegment", PromptVersion: "test", Outcome: "success"}
	if err := repo.RecordLLMCall(ctx, call); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, c.ID, c.Revision-1, at); !errors.Is(err, campaign.ErrRevision) {
		t.Fatal("stale delete accepted", err)
	}
	if err := repo.Delete(ctx, c.ID, c.Revision, at); err != nil {
		t.Fatal(err)
	}
	// An already-running generation must still be able to finish its audit.
	if err := repo.RecordLLMCall(ctx, call); err != nil {
		t.Fatal("late audit lost", err)
	}
	var retained bool
	if err := repo.DB.QueryRowContext(ctx, `SELECT deleted_at=$2 AND revision=$3 AND status='cancelled'
      AND (SELECT count(*) FROM app.llm_calls WHERE campaign_id=$1)=2
      AND (SELECT count(*) FROM app.campaign_messages WHERE campaign_id=$1)=2
      FROM app.campaigns WHERE id=$1`, c.ID, at, c.Revision+1).Scan(&retained); err != nil || !retained {
		t.Fatal("content/audit not retained", err)
	}
	page, err := repo.List(ctx, 0, 100)
	if err != nil || len(page.Campaigns) != 0 {
		t.Fatal("deleted campaign listed", err)
	}
	v, err := segment.ParseDocument(c.Segment)
	if err != nil {
		t.Fatal(err)
	}
	checks := []func() error{
		func() error { _, e := repo.Get(ctx, c.ID); return e },
		func() error { _, e := repo.GetMessages(ctx, c.ID); return e },
		func() error { _, e := repo.GetDeliveryStatus(ctx, c.ID); return e },
		func() error { _, e := repo.ApprovalPreview(ctx, c.ID, at); return e },
		func() error { _, _, e := repo.GenerationInput(ctx, c.ID, at); return e },
		func() error { _, e := repo.PutSegment(ctx, c.ID, c.Revision+1, v); return e },
		func() error { _, e := repo.PutMessages(ctx, c.ID, c.Revision+1, messageBody("Edit"), at); return e },
		func() error {
			_, e := repo.Approve(ctx, c.ID, c.Revision+1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", at)
			return e
		},
		func() error { return repo.Delete(ctx, c.ID, c.Revision, at) },
	}
	for i, check := range checks {
		if e := check(); !errors.Is(e, campaign.ErrNotFound) {
			t.Fatalf("operation %d did not hide tombstone: %v", i, e)
		}
	}
	draft, err := repo.Create(ctx, campaign.Draft{Brief: "empty draft", Channels: []string{"sms"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, draft.ID, draft.Revision, at); err != nil {
		t.Fatal("empty draft deletion", err)
	}
}

func TestDeleteRejectsLifecycleAndHistory(t *testing.T) {
	for _, state := range []string{"approved", "sending", "completed", "cancelled", "approval_timestamp", "delivery_history"} {
		t.Run(state, func(t *testing.T) {
			repo, ctx, c, at := approvalFixture(t)
			if state == "sending" || state == "completed" || state == "delivery_history" {
				p, err := repo.ApprovalPreview(ctx, c.ID, at)
				if err != nil {
					t.Fatal(err)
				}
				result, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
				if err != nil {
					t.Fatal(err)
				}
				c.Revision = result.Revision
				if state == "completed" {
					if _, err := repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET status='completed' WHERE id=$1`, c.ID); err != nil {
						t.Fatal(err)
					}
				}
				if state == "delivery_history" {
					// Corrupt lifecycle metadata must not permit deleting queue history.
					if _, err := repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET status='draft',approved_at=NULL,approved_revision=NULL,approval_fingerprint=NULL,approval_audience_preview=NULL WHERE id=$1`, c.ID); err != nil {
						t.Fatal(err)
					}
				}
			} else if state == "approval_timestamp" {
				if _, err := repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET approved_at=$2 WHERE id=$1`, c.ID, at); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET status=$2 WHERE id=$1`, c.ID, state); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.Delete(ctx, c.ID, c.Revision, at); !errors.Is(err, campaign.ErrState) {
				t.Fatal("unsafe deletion accepted", err)
			}
			if _, err := repo.Get(ctx, c.ID); err != nil {
				t.Fatal("rejected deletion hid campaign", err)
			}
			if state == "sending" || state == "completed" || state == "delivery_history" {
				var n int
				if err := repo.DB.QueryRowContext(ctx, `SELECT count(*) FROM app.deliveries WHERE campaign_id=$1`, c.ID).Scan(&n); err != nil || n != 3 {
					t.Fatal("delivery history changed", err)
				}
			}
		})
	}
}

func TestDeleteRacesApprovalAndSave(t *testing.T) {
	for _, operation := range []string{"approve", "save"} {
		t.Run(operation, func(t *testing.T) {
			repo, ctx, c, at := approvalFixture(t)
			p, err := repo.ApprovalPreview(ctx, c.ID, at)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() { <-start; results <- repo.Delete(ctx, c.ID, c.Revision, at) }()
			go func() {
				<-start
				var e error
				if operation == "approve" {
					_, e = repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
				} else {
					_, e = repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Changed"), at)
				}
				results <- e
			}()
			close(start)
			first, second := <-results, <-results
			if (first == nil) == (second == nil) {
				t.Fatal("expected exactly one mutation to succeed", first, second)
			}
			loser := first
			if loser == nil {
				loser = second
			}
			if !errors.Is(loser, campaign.ErrNotFound) && !errors.Is(loser, campaign.ErrRevision) && !errors.Is(loser, campaign.ErrConcurrentUpdate) {
				t.Fatal("unexpected race failure", loser)
			}
			var valid bool
			if err := repo.DB.QueryRowContext(ctx, `SELECT (deleted_at IS NULL OR NOT EXISTS(SELECT 1 FROM app.deliveries WHERE campaign_id=$1)) FROM app.campaigns WHERE id=$1`, c.ID).Scan(&valid); err != nil || !valid {
				t.Fatal("deleted campaign has deliveries", err)
			}
		})
	}
}
