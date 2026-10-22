//go:build integration

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/campaign"
	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
	"github.com/KayaOrhan/SmartCampaignComposer/seed"
)

func approvalFixture(t *testing.T) (Campaigns, context.Context, campaign.Campaign, time.Time) {
	t.Helper()
	repo, ctx, c, at := messageRepo(t)
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) VALUES
        (1,'Ali','Example','fixture1@example.test','mock-phone-000001','Istanbul','istanbul','tr',$1),
        (2,'Ece','Example','fixture2@example.test','mock-phone-000002','Istanbul','istanbul','tr',$1),
        (3,'Ada','Example','fixture3@example.test','mock-phone-000003','Istanbul','istanbul','tr',$1)`, at); err != nil {
		t.Fatal("user fixture failed")
	}
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.consents(user_id,channel,granted,updated_at) VALUES (1,'sms',true,$1),(1,'email',true,$1),(2,'email',true,$1),(2,'sms',false,$1)`, at); err != nil {
		t.Fatal("consent fixture failed")
	}
	body := []byte(`{"sms":{"body":"Hello {{first_name}} in {{city}}"},"email":{"subject":"Hi {{first_name}}","body":"Visit {{city}}"}}`)
	if _, err := repo.PutMessages(ctx, c.ID, c.Revision, body, at); err != nil {
		t.Fatal(err)
	}
	c, err := repo.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, ctx, c, at
}

func assertNoApproval(t *testing.T, repo Campaigns, ctx context.Context, c campaign.Campaign) {
	t.Helper()
	state, err := repo.GetDeliveryStatus(ctx, c.ID)
	if err != nil || state.Status != c.Status || state.Revision != c.Revision || state.ApprovedAt != nil || state.Counts.Total != 0 {
		t.Fatal("rejected approval changed campaign or queue", err)
	}
}

func TestApprovalQueueSnapshotAndRepeat(t *testing.T) {
	repo, ctx, c, at := approvalFixture(t)
	p, err := repo.ApprovalPreview(ctx, c.ID, at)
	if err != nil || p.ApprovalFingerprint == nil || p.Revision != c.Revision {
		t.Fatal("approval preview failed", err)
	}
	later, err := repo.ApprovalPreview(ctx, c.ID, at.Add(time.Minute))
	if err != nil || later.ApprovalFingerprint == nil || *p.ApprovalFingerprint != *later.ApprovalFingerprint {
		t.Fatal("timestamp changed fingerprint", err)
	}
	result, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
	if err != nil || result.Status != "sending" || result.Revision != c.Revision+1 || result.Counts.Pending != 3 || result.Counts.Total != 3 || result.ByChannel["sms"].Pending != 1 || result.ByChannel["email"].Pending != 2 || result.ApprovedRevision == nil || *result.ApprovedRevision != c.Revision || result.ApprovedAt == nil || !result.ApprovedAt.Equal(at) {
		t.Fatal("approval/queue mismatch", err)
	}
	var valid bool
	if err := repo.DB.QueryRowContext(ctx, `SELECT count(*)=3 AND bool_and(status='pending' AND attempts=0 AND claim_generation=0 AND locked_at IS NULL AND lease_expires_at IS NULL AND provider_message_id IS NULL AND last_error_code IS NULL) FROM app.deliveries WHERE campaign_id=$1`, c.ID).Scan(&valid); err != nil || !valid {
		t.Fatal("initial queue state invalid")
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT count(*)=1 FROM app.deliveries WHERE campaign_id=$1 AND user_id=1 AND channel='sms' AND subject IS NULL AND body='Hello Ali in Istanbul' AND destination='mock-phone-000001'`, c.ID).Scan(&valid); err != nil || !valid {
		t.Fatal("SMS not correctly frozen")
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT count(*)=1 FROM app.deliveries WHERE campaign_id=$1 AND user_id=1 AND channel='email' AND subject='Hi Ali' AND body='Visit Istanbul' AND destination='fixture1@example.test'`, c.ID).Scan(&valid); err != nil || !valid {
		t.Fatal("email not correctly frozen")
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.users SET first_name='Changed',city='Changed',email='changed@example.test',phone='mock-phone-changed' WHERE id=1; UPDATE app.consents SET granted=false WHERE user_id=1`); err != nil {
		t.Fatal("profile mutation failed")
	}
	repeated, err := repo.Approve(ctx, c.ID, result.Revision, *p.ApprovalFingerprint, at.Add(time.Hour))
	if err != nil || !reflect.DeepEqual(repeated, result) {
		t.Fatal("repeat changed approval", err)
	}
	if _, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at); !errors.Is(err, campaign.ErrRevision) {
		t.Fatal("old revision accepted", err)
	}
	if err := repo.DB.QueryRowContext(ctx, `SELECT count(*)=1 FROM app.deliveries WHERE campaign_id=$1 AND user_id=1 AND channel='sms' AND body='Hello Ali in Istanbul' AND destination='mock-phone-000001'`, c.ID).Scan(&valid); err != nil || !valid {
		t.Fatal("frozen payload changed")
	}
	if _, err := repo.PutMessages(ctx, c.ID, result.Revision, messageBody("Edit"), at); !errors.Is(err, campaign.ErrState) {
		t.Fatal("approved templates editable", err)
	}
	v, err := segment.ParseDocument(c.Segment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutSegment(ctx, c.ID, result.Revision, v); !errors.Is(err, campaign.ErrState) {
		t.Fatal("approved segment editable", err)
	}
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.deliveries(campaign_id,user_id,channel,destination,subject,body,idempotency_key) SELECT campaign_id,user_id,channel,destination,subject,body,idempotency_key FROM app.deliveries WHERE campaign_id=$1 AND user_id=1 AND channel='sms'`, c.ID); err == nil {
		t.Fatal("duplicate delivery accepted")
	}
	if _, err := repo.DB.ExecContext(ctx, `UPDATE app.deliveries SET status=CASE WHEN channel='sms' THEN 'skipped' WHEN user_id=1 THEN 'sent' ELSE 'dead' END WHERE campaign_id=$1`, c.ID); err != nil {
		t.Fatal("result fixture failed")
	}
	status, err := repo.GetDeliveryStatus(ctx, c.ID)
	if err != nil || status.Counts.Total != 3 || status.Counts.Pending != 0 || status.Counts.Sent != 1 || status.Counts.Dead != 1 || status.Counts.Skipped != 1 || status.Revision != result.Revision {
		t.Fatal("database counts incorrect", err)
	}
	encoded, _ := json.Marshal(status)
	for _, forbidden := range []string{"@example.test", "mock-phone", "Hello Ali", "Istanbul", "destination", "body"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatal("recipient data in progress response")
		}
	}
}

func TestApprovalDetectsSameCountChanges(t *testing.T) {
	for _, change := range []string{"name", "email", "phone", "membership"} {
		t.Run(change, func(t *testing.T) {
			repo, ctx, c, at := approvalFixture(t)
			before, err := repo.ApprovalPreview(ctx, c.ID, at)
			if err != nil {
				t.Fatal(err)
			}
			query := map[string]string{
				"name":       `UPDATE app.users SET first_name='Eli' WHERE id=1`,
				"email":      `UPDATE app.users SET email='other@example.test' WHERE id=1`,
				"phone":      `UPDATE app.users SET phone='mock-phone-other' WHERE id=1`,
				"membership": `UPDATE app.consents SET granted=NOT granted WHERE channel='sms' AND user_id IN (1,2)`,
			}[change]
			if _, err := repo.DB.ExecContext(ctx, query); err != nil {
				t.Fatal("change fixture failed")
			}
			_, err = repo.Approve(ctx, c.ID, c.Revision, *before.ApprovalFingerprint, at)
			var changed *campaign.ApprovalPreviewChanged
			if !errors.As(err, &changed) || changed.Preview.ApprovalFingerprint == nil || *changed.Preview.ApprovalFingerprint == *before.ApprovalFingerprint || !reflect.DeepEqual(before.ByChannel, changed.Preview.ByChannel) || before.EligibleUnique != changed.Preview.EligibleUnique {
				t.Fatal("same-count change not detected", err)
			}
			assertNoApproval(t, repo, ctx, c)
			if _, err := repo.Approve(ctx, c.ID, c.Revision, *changed.Preview.ApprovalFingerprint, at); err != nil {
				t.Fatal("explicitly reviewed snapshot rejected", err)
			}
		})
	}
}

func TestApprovalRejectsInvalidCurrentState(t *testing.T) {
	for _, scenario := range []string{"draft", "empty", "length", "missing template", "stale", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			repo, ctx, c, at := approvalFixture(t)
			p, err := repo.ApprovalPreview(ctx, c.ID, at)
			if err != nil {
				t.Fatal(err)
			}
			expected := c.Revision
			switch scenario {
			case "draft":
				v, _ := segment.ParseDocument(c.Segment)
				c, err = repo.PutSegment(ctx, c.ID, c.Revision, v)
				expected = c.Revision
			case "empty":
				_, err = repo.DB.ExecContext(ctx, `UPDATE app.consents SET granted=false`)
			case "length":
				_, err = repo.DB.ExecContext(ctx, `UPDATE app.users SET first_name=$1 WHERE id=1`, strings.Repeat("x", 80))
			case "missing template":
				_, err = repo.DB.ExecContext(ctx, `DELETE FROM app.campaign_messages WHERE campaign_id=$1 AND channel='email'`, c.ID)
			case "stale":
				_, err = repo.PutMessages(ctx, c.ID, c.Revision, messageBody("New"), at)
				c, err = repo.Get(ctx, c.ID)
			case "cancelled":
				_, err = repo.DB.ExecContext(ctx, `UPDATE app.campaigns SET status='cancelled' WHERE id=$1`, c.ID)
				c.Status = "cancelled"
			}
			if err != nil {
				t.Fatal("state fixture failed")
			}
			_, err = repo.Approve(ctx, c.ID, expected, *p.ApprovalFingerprint, at)
			if err == nil {
				t.Fatal("invalid approval accepted")
			}
			if scenario == "stale" && !errors.Is(err, campaign.ErrRevision) {
				t.Fatal("stale precondition lost", err)
			}
			if scenario == "empty" && !errors.Is(err, campaign.ErrEmptyAudience) {
				t.Fatal("empty audience accepted", err)
			}
			if scenario == "length" {
				var v *segment.ValidationError
				if !errors.As(err, &v) || v.Code != "message_too_long" {
					t.Fatal("current bounds not validated", err)
				}
			}
			assertNoApproval(t, repo, ctx, c)
		})
	}
}

func TestApprovalRollbackAcrossBatches(t *testing.T) {
	repo, ctx, c, at := approvalFixture(t)
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.users(id,first_name,last_name,email,phone,city,city_norm,language,created_at) SELECT n,'A','Example','bulk-'||n::text||'@example.test','mock-phone-'||n::text,'X','x','tr',$1 FROM generate_series(4,205) n`, at); err != nil {
		t.Fatal("bulk users failed")
	}
	if _, err := repo.DB.ExecContext(ctx, `INSERT INTO app.consents(user_id,channel,granted,updated_at) SELECT id,'sms',true,$1 FROM app.users WHERE id>=4`, at); err != nil {
		t.Fatal("bulk consent failed")
	}
	p, err := repo.ApprovalPreview(ctx, c.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB.ExecContext(ctx, `ALTER TABLE app.deliveries ADD CONSTRAINT reject_late_test_row CHECK (user_id<>205)`); err != nil {
		t.Fatal("failure constraint failed")
	}
	_, err = repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal("insertion failure not surfaced", err)
	}
	assertNoApproval(t, repo, ctx, c)
	if _, err := repo.DB.ExecContext(ctx, `ALTER TABLE app.deliveries DROP CONSTRAINT reject_late_test_row`); err != nil {
		t.Fatal("constraint cleanup failed")
	}
	if _, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at); err != nil {
		t.Fatal("approval could not recover after rollback", err)
	}
}

func TestApprovalConcurrency(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "approve twice", true: "edit versus approve"}[edit], func(t *testing.T) {
			repo, ctx, c, at := approvalFixture(t)
			p, err := repo.ApprovalPreview(ctx, c.ID, at)
			if err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
				results <- err
			}()
			go func() {
				<-start
				var err error
				if edit {
					_, err = repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Edited"), at)
				} else {
					_, err = repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
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
			status, err := repo.GetDeliveryStatus(ctx, c.ID)
			if err != nil || wins != 1 || conflicts != 1 || status.Revision != c.Revision+1 {
				t.Fatal("approval concurrency failed", err)
			}
			if status.Status == "sending" && status.Counts.Total != 3 {
				t.Fatal("partial/duplicate queue")
			}
			if status.Status == "content_ready" && status.Counts.Total != 0 {
				t.Fatal("edit winner has queued deliveries")
			}
		})
	}
}

func TestApprovalSeedVolumeAndCancellation(t *testing.T) {
	repo, ctx, c, at := messageRepo(t)
	if err := seed.Ensure(ctx, repo.DB, at); err != nil {
		t.Fatal("seed failed")
	}
	v, err := segment.Parse([]byte(`{"version":1,"all":[{"field":"language","op":"in","value":["tr","en"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err = repo.PutSegment(ctx, c.ID, c.Revision, v)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PutMessages(ctx, c.ID, c.Revision, messageBody("Hello {{first_name}}"), at); err != nil {
		t.Fatal(err)
	}
	c, err = repo.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	repo.Timeout = 2 * time.Second
	p, err := repo.ApprovalPreview(ctx, c.ID, at)
	if err != nil || p.EligibleUnique < 1000 {
		t.Fatal("seed preview failed", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.Approve(cancelled, c.ID, c.Revision, *p.ApprovalFingerprint, at); err == nil {
		t.Fatal("cancelled approval succeeded")
	}
	assertNoApproval(t, repo, ctx, c)
	approved, err := repo.Approve(ctx, c.ID, c.Revision, *p.ApprovalFingerprint, at)
	if err != nil || approved.Counts.Total != int64(p.ByChannel["sms"].Count+p.ByChannel["email"].Count) {
		t.Fatal("seed approval failed", err)
	}
}
