package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func TestRevisionPreconditions(t *testing.T) {
	for _, tc := range []struct{ value, code string }{
		{"", "revision_required"}, {"*", "invalid_revision"}, {`W/"campaign-1-r1"`, "invalid_revision"},
		{`"campaign-2-r1"`, "invalid_revision"}, {`"campaign-1-r01"`, "invalid_revision"},
		{`"campaign-1-r0"`, "invalid_revision"}, {`"campaign-1-r9223372036854775808"`, "invalid_revision"},
		{`"campaign-1-r1", "campaign-1-r2"`, "invalid_revision"}, {`"campaign-1-r7"`, ""},
	} {
		r := httptest.NewRequest("PUT", "/campaigns/1/segment", nil)
		if tc.value != "" {
			r.Header.Set("If-Match", tc.value)
		}
		n, err := requiredRevision(r, 1)
		if tc.code == "" {
			if err != nil || n != 7 {
				t.Fatal("valid revision rejected", err)
			}
			continue
		}
		v, ok := err.(*segment.ValidationError)
		if !ok || v.Code != tc.code {
			t.Fatal("wrong precondition error", err)
		}
		w := httptest.NewRecorder()
		fail(w, err)
		want := 400
		if tc.value == "" {
			want = 428
		}
		if w.Code != want {
			t.Fatal("wrong precondition status", w.Code)
		}
	}
}
