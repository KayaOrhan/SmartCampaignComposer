//go:build integration

package store

import (
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/llm"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func TestMetricsAggregates(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	s := Campaigns{DB: db, Timeout: time.Second}
	m, err := s.Metrics(ctx)
	if err != nil || len(m.Deliveries) != 2 || m.Deliveries["sms"].Total != 0 {
		t.Fatal("empty metrics")
	}
	c, err := s.Create(ctx, campaign.Draft{Brief: "Metrics fixture", Channels: []string{"sms"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		err = s.RecordLLMCall(ctx, llm.Call{CampaignID: c.ID, Name: "ExtractSegment", PromptVersion: "test", Outcome: "llm_output_invalid", Errors: []segment.ValidationError{{Code: "unknown_field", Path: "all"}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	m, err = s.Metrics(ctx)
	if err != nil || len(m.Calls) != 1 || m.Calls[0].Count != 2 || m.Calls[0].ValidationFailures != 2 {
		t.Fatal("audit counters do not match persisted attempts")
	}
}

func TestMetricsDeliveryStates(t *testing.T) {
	repo, _, ctx, c := queuedFixture(t)
	m, err := repo.Metrics(ctx)
	if err != nil || m.Deliveries["sms"].Pending != 1 || m.Deliveries["email"].Pending != 2 {
		t.Fatal("pending queue metrics")
	}
	if _, err = repo.DB.ExecContext(ctx, `UPDATE app.deliveries SET status=CASE WHEN channel='sms' THEN 'skipped' WHEN user_id=1 THEN 'sent' ELSE 'dead' END WHERE campaign_id=$1`, c.ID); err != nil {
		t.Fatal("metrics fixture update failed")
	}
	m, err = repo.Metrics(ctx)
	if err != nil || m.Deliveries["sms"].Skipped != 1 || m.Deliveries["email"].Sent != 1 || m.Deliveries["email"].Dead != 1 || m.Deliveries["email"].Pending != 0 {
		t.Fatal("terminal metrics")
	}
}
