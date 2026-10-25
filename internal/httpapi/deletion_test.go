package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeleteRequiresRevision(t *testing.T) {
	for _, token := range []string{"", "*", `"campaign-2-r1"`, `W/"campaign-1-r1"`} {
		store := &fakeCampaignStore{}
		mux := http.NewServeMux()
		registerCampaigns(mux, store, func() time.Time { return time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC) }, time.Second)
		r := httptest.NewRequest("DELETE", "/campaigns/1", nil)
		if token != "" {
			r.Header.Set("If-Match", token)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		want := 400
		if token == "" {
			want = 428
		}
		if w.Code != want || store.calls != 0 {
			t.Fatalf("token %q: status=%d calls=%d", token, w.Code, store.calls)
		}
	}
}
