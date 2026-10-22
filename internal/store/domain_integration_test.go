//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
	"github.com/KayaOrhan/SmartCampaignComposer/seed"
)

func TestDomain(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for i := 1; i <= 10; i++ {
		city, first := "Istanbul", "Ali"
		if i == 1 {
			city = "  Istanbul  "
		}
		if i == 2 {
			city = "İstanbul"
			first = "Abdülkadir"
		}
		if i == 3 {
			city = "ISTANBUL"
			first = "Deniz"
		}
		if i == 8 {
			city = "Ankara"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) VALUES ($1,$2,'Example',$3,'mock-phone',$4,$5,'tr',$6)`, i, first, fmt.Sprintf("fixture%d@example.test", i), city, segment.NormalizeCity(city), asOf.Add(-100*24*time.Hour)); err != nil {
			t.Fatal("insert user fixture failed")
		}
		for _, channel := range []string{"sms", "email"} {
			if i == 3 && channel == "sms" {
				continue
			}
			granted := !(i == 2 && channel == "email") && !(i == 4 && channel == "sms")
			if _, err := db.ExecContext(ctx, `INSERT INTO app.consents VALUES ($1,$2,$3,$4)`, i, channel, granted, asOf); err != nil {
				t.Fatal("insert consent fixture failed")
			}
		}
		cart := asOf.Add(-24 * time.Hour)
		if i == 6 {
			cart = asOf.Add(-31 * 24 * time.Hour)
		}
		if i == 7 {
			cart = asOf
		}
		if i == 9 {
			cart = asOf.Add(-30 * 24 * time.Hour)
		}
		if i == 10 {
			cart = asOf.Add(-30*24*time.Hour - time.Microsecond)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO app.cart_events(user_id,product_id,added_at) VALUES ($1,1,$2),($1,2,$2)`, i, cart); err != nil {
			t.Fatal("insert cart fixture failed")
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.orders(user_id,placed_at,total) VALUES (5,$1,100),(9,$2,100),(1,$3,100)`, asOf.Add(-30*24*time.Hour), asOf, asOf.Add(-30*24*time.Hour-time.Microsecond)); err != nil {
		t.Fatal("insert order fixtures failed")
	}
	repo := Campaigns{DB: db, Timeout: 5 * time.Second}
	c, err := repo.Create(ctx, campaign.Draft{Brief: "example brief", Channels: []string{"email", "sms"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Preview(ctx, c.ID, asOf); !errors.Is(err, campaign.ErrSegmentRequired) {
		t.Fatal("missing segment did not fail closed")
	}
	v, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"city","op":"in","value":["ISTANBUL"]},{"field":"cart_added","op":"within_days","value":30},{"field":"purchase","op":"none_within_days","value":30}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, v); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, v); err != nil {
		t.Fatal("repeat segment update:", err)
	}
	stored, err := repo.Get(ctx, c.ID)
	if err != nil || string(stored.Segment) != string(v.JSON()) || stored.Status != "draft" {
		t.Fatal("draft round trip failed")
	}
	p, err := repo.Preview(ctx, c.ID, asOf)
	if err != nil {
		t.Fatal(err)
	}
	if p.EligibleUnique != 5 || p.ByChannel["sms"].Count != 3 || p.ByChannel["email"].Count != 4 {
		t.Fatalf("wrong consent/time/deduplication result: %+v", p)
	}
	if p.ByChannel["sms"].MaxFirstName != 10 || p.ByChannel["sms"].MaxCity != 12 {
		t.Fatal("both independent Unicode bounds not computed")
	}
	injected, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"city","op":"in","value":["'; DROP TABLE app.users; --"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, injected); err != nil {
		t.Fatal(err)
	}
	injectedPreview, err := repo.Preview(ctx, c.ID, asOf)
	if err != nil || injectedPreview.EligibleUnique != 0 {
		t.Fatal("injection fixture was not treated as a literal value")
	}
	var usersRemain int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.users`).Scan(&usersRemain); err != nil || usersRemain != 10 {
		t.Fatal("injection fixture affected user data")
	}
	recent, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"purchase","op":"within_days","value":30}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, recent); err != nil {
		t.Fatal(err)
	}
	recentPreview, err := repo.Preview(ctx, c.ID, asOf)
	if err != nil || recentPreview.EligibleUnique != 1 {
		t.Fatal("purchase window boundaries incorrect")
	}
	if _, err := repo.PutSegment(ctx, c.ID, v); err != nil {
		t.Fatal(err)
	}
	smsOnly, err := repo.Create(ctx, campaign.Draft{Brief: "single channel", Channels: []string{"sms"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, smsOnly.ID, v); err != nil {
		t.Fatal(err)
	}
	single, err := repo.Preview(ctx, smsOnly.ID, asOf)
	if err != nil || len(single.ByChannel) != 1 || single.EligibleUnique != 3 {
		t.Fatal("selected channels not respected")
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.consents SET granted=false WHERE user_id=2 AND channel='sms'`); err != nil {
		t.Fatal("revoke consent failed")
	}
	p, err = repo.Preview(ctx, c.ID, asOf)
	if err != nil || p.EligibleUnique != 4 || p.ByChannel["sms"].Count != 2 {
		t.Fatal("revocation not respected")
	}
	empty, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"language","op":"in","value":["fr"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, empty); err != nil {
		t.Fatal(err)
	}
	p, err = repo.Preview(ctx, c.ID, asOf)
	if err != nil || p.EligibleUnique != 0 || p.ByChannel["sms"] != (campaign.Bounds{}) {
		t.Fatal("empty audience not represented correctly")
	}
	if _, err := repo.Get(ctx, 99999); !errors.Is(err, campaign.ErrNotFound) {
		t.Fatal("missing campaign not classified")
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.campaigns SET status='approved' WHERE id=$1`, c.ID); err != nil {
		t.Fatal("state fixture failed")
	}
	if _, err := repo.PutSegment(ctx, c.ID, v); !errors.Is(err, campaign.ErrState) {
		t.Fatal("non-draft segment editable")
	}
	if _, err := repo.Preview(ctx, c.ID, asOf); !errors.Is(err, campaign.ErrState) {
		t.Fatal("non-draft preview accepted")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO app.consents VALUES (1,'push',true,$1)`, asOf); err == nil {
		t.Fatal("database accepted unknown channel")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.Get(cancelled, c.ID); err == nil {
		t.Fatal("cancelled query succeeded")
	}
}

func TestSeedFailureRollsBack(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	if _, err := db.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) VALUES (2,'Example','Example','fixture@example.test','mock-phone','Ankara','ankara','tr',$1)`, at); err != nil {
		t.Fatal("insert conflicting seed fixture failed")
	}
	if err := seed.Ensure(ctx, db, at); err == nil {
		t.Fatal("conflicting seed unexpectedly succeeded")
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.users`).Scan(&count); err != nil || count != 1 {
		t.Fatal("partial seed rows were committed")
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.seed_runs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed seed was marked complete")
	}
}

func TestSeedPersistence(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	if err := seed.Ensure(ctx, db, asOf); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE app.consents SET granted=false WHERE user_id=1 AND channel='sms'`); err != nil {
		t.Fatal("revoke seeded consent failed")
	}
	if err := seed.Ensure(ctx, db, asOf.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM app.users`).Scan(&count); err != nil || count != seed.Population {
		t.Fatal("seed duplicated or incomplete")
	}
	var consent bool
	if err := db.QueryRowContext(ctx, `SELECT granted FROM app.consents WHERE user_id=1 AND channel='sms'`).Scan(&consent); err != nil || consent {
		t.Fatal("seed overwrote revocation")
	}
	var reference time.Time
	if err := db.QueryRowContext(ctx, `SELECT reference_time FROM app.seed_runs WHERE version=1`).Scan(&reference); err != nil || !reference.Equal(asOf) {
		t.Fatal("seed anchor changed")
	}
}

func TestSegmentPersistenceRoundTrip(t *testing.T) {
	db, ctx := testDatabase(t)
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	repo := Campaigns{DB: db, Timeout: 5 * time.Second}
	for _, tc := range []struct {
		name, value    string
		jsonbExpansion bool
	}{
		{"canonical escaping", strings.Repeat("<", 120), false},
		{"JSONB whitespace", strings.Repeat("界", 66) + "a", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := make([]string, 20)
			for i := range values {
				values[i] = tc.value
			}
			conditions := make([]map[string]any, 8)
			for i := range conditions {
				conditions[i] = map[string]any{"field": "city", "op": "in", "value": values}
			}
			var input bytes.Buffer
			enc := json.NewEncoder(&input)
			enc.SetEscapeHTML(false)
			if err := enc.Encode(map[string]any{"version": 1, "all": conditions}); err != nil {
				t.Fatal(err)
			}
			v, err := segment.Parse(input.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			c, err := repo.Create(ctx, campaign.Draft{Brief: "round trip fixture", Channels: []string{"sms", "email"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.PutSegment(ctx, c.ID, v); err != nil {
				t.Fatal("initial save", err)
			}
			if tc.jsonbExpansion {
				var size int
				if err := db.QueryRowContext(ctx, `SELECT octet_length(segment_json::text) FROM app.campaigns WHERE id=$1`, c.ID).Scan(&size); err != nil {
					t.Fatal(err)
				}
				if len(v.JSON()) > segment.MaxJSONBytes || size <= segment.MaxJSONBytes {
					t.Fatalf("fixture must cross limit only after JSONB: canonical=%d stored=%d", len(v.JSON()), size)
				}
			}
			stored, err := repo.Get(ctx, c.ID)
			if err != nil || !bytes.Equal(stored.Segment, v.JSON()) {
				t.Fatal("read failed", err)
			}
			p, err := repo.Preview(ctx, c.ID, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC))
			if err != nil || p.EligibleUnique != 0 || len(p.ByChannel) != 2 {
				t.Fatal("preview failed", err)
			}
			if _, err := repo.PutSegment(ctx, c.ID, v); err != nil {
				t.Fatal("repeat save", err)
			}
		})
	}
}
