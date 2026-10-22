package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

func (a campaignAPI) list(w http.ResponseWriter, r *http.Request) {
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(w, segment.Invalid("invalid_pagination", "$"))
		return
	}
	var before int64
	limit := int64(20)
	for key, values := range q {
		if (key != "before" && key != "limit") || len(values) != 1 {
			fail(w, segment.Invalid("invalid_pagination", "$"))
			return
		}
		n, parseErr := strconv.ParseInt(values[0], 10, 64)
		if parseErr != nil || n < 1 || strconv.FormatInt(n, 10) != values[0] {
			err = segment.Invalid("invalid_pagination", key)
			break
		}
		if key == "before" {
			before = n
		} else {
			limit = n
		}
	}
	if err != nil {
		fail(w, err)
		return
	}
	if limit > 100 {
		fail(w, segment.Invalid("invalid_pagination", "limit"))
		return
	}
	result, err := a.store.List(r.Context(), before, int(limit))
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, result)
}
