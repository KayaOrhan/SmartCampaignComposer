package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListPagination(t *testing.T) {
	for _, query := range []string{"", "?limit=1&before=100", "?limit=100", "?limit=0", "?limit=101", "?before=-1", "?before=01", "?limit=1&limit=2", "?unknown=1", "?before=9223372036854775808", "?limit=2;before=3", "?before=%zz"} {
		t.Run(query, func(t *testing.T) {
			s := &fakeCampaignStore{}
			mux := http.NewServeMux()
			registerCampaigns(mux, s, time.Now, time.Second)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", "/campaigns"+query, nil))
			valid := query == "" || query == "?limit=1&before=100" || query == "?limit=100"
			if valid {
				if w.Code != 200 || !s.deadline || w.Body.String() != "{\"campaigns\":[],\"next_before\":null}\n" {
					t.Fatalf("unexpected page: %d %s", w.Code, w.Body)
				}
			} else if w.Code != 422 || s.calls != 0 {
				t.Fatalf("invalid cursor reached storage: %d", w.Code)
			}
		})
	}
}
