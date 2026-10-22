//go:build integration

package store

import (
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
)

func TestCampaignList(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	s := Campaigns{DB: db, Timeout: time.Second}
	p, err := s.List(ctx, 0, 2)
	if err != nil || p.Campaigns == nil || len(p.Campaigns) != 0 || p.NextBefore != nil {
		t.Fatal("empty page contract")
	}
	var ids []int64
	for i := 0; i < 3; i++ {
		c, err := s.Create(ctx, campaign.Draft{Brief: "List fixture", Channels: []string{"sms"}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	p, err = s.List(ctx, 0, 2)
	if err != nil || len(p.Campaigns) != 2 || p.Campaigns[0].ID != ids[2] || p.NextBefore == nil || *p.NextBefore != ids[1] {
		t.Fatal("first page ordering")
	}
	if _, err = s.Create(ctx, campaign.Draft{Brief: "Concurrent creation", Channels: []string{"email"}}); err != nil {
		t.Fatal(err)
	}
	p, err = s.List(ctx, *p.NextBefore, 2)
	if err != nil || len(p.Campaigns) != 1 || p.Campaigns[0].ID != ids[0] || p.NextBefore != nil {
		t.Fatal("cursor shifted after creation")
	}
}
