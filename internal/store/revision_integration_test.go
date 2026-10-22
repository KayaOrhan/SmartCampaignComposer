//go:build integration

package store

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
	"github.com/KayaOrhan/SmartCampaignComposer/migrations"
)

func TestCampaignUpgradePreservesDraft(t *testing.T) {
	db, ctx := testDatabase(t)
	previous := fstest.MapFS{}
	for _, name := range []string{"001_init.sql", "002_campaign_domain.sql", "003_llm_calls.sql", "004_llm_message_calls.sql"} {
		body, err := fs.ReadFile(migrations.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		previous[name] = &fstest.MapFile{Data: body}
	}
	if err := migrate(ctx, db, previous, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := db.QueryRowContext(ctx, `INSERT INTO app.campaigns(brief,channels) VALUES ('upgrade fixture',ARRAY['sms']) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	repo := Campaigns{DB: db, Timeout: 5 * time.Second}
	c, err := repo.Get(ctx, id)
	if err != nil || c.Revision != 1 || c.Brief != "upgrade fixture" || c.Status != "draft" {
		t.Fatal("upgrade lost draft", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.campaigns SET status='content_ready' WHERE id=$1`, id); err == nil {
		t.Fatal("ready without validated content accepted")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.campaign_messages(campaign_id,channel,body) VALUES ($1,'push','Body')`, id); err == nil {
		t.Fatal("unknown channel accepted")
	}
}

func TestSegmentRevisionConcurrency(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	repo := Campaigns{DB: db, Timeout: 5 * time.Second}
	c, err := repo.Create(ctx, campaign.Draft{Brief: "revision fixture", Channels: []string{"sms"}})
	if err != nil || c.Revision != 1 {
		t.Fatal("initial revision", err)
	}
	v, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"language","op":"in","value":["tr"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := repo.PutSegment(ctx, c.ID, 1, v); results <- err }()
	}
	close(start)
	success, stale := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, campaign.ErrRevision) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	stored, err := repo.Get(ctx, c.ID)
	if err != nil || success != 1 || stale != 1 || stored.Revision != 2 {
		t.Fatal("lost update protection failed", err)
	}
}
