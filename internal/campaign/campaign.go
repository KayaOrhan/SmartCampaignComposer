package campaign

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/segment"
)

var ErrNotFound = errors.New("campaign not found")
var ErrState = errors.New("campaign state does not allow this operation")
var ErrSegmentRequired = errors.New("campaign segment required")
var ErrRevision = errors.New("campaign revision conflict")
var ErrConcurrentUpdate = errors.New("concurrent campaign update")
var ErrEmptyAudience = errors.New("campaign audience is empty")

func Editable(status string) bool { return status == "draft" || status == "content_ready" }

type Draft struct {
	Brief    string   `json:"brief"`
	Channels []string `json:"channels"`
}
type Campaign struct {
	ID        int64           `json:"id"`
	Revision  int64           `json:"revision"`
	Brief     string          `json:"brief"`
	Channels  []string        `json:"channels"`
	Segment   json.RawMessage `json:"segment"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
}

func (d Draft) Validate() (Draft, error) {
	d.Brief = strings.TrimSpace(d.Brief)
	if !utf8.ValidString(d.Brief) || utf8.RuneCountInString(d.Brief) < 1 || utf8.RuneCountInString(d.Brief) > 4000 || strings.ContainsRune(d.Brief, 0) {
		return Draft{}, segment.Invalid("invalid_brief", "brief")
	}
	if len(d.Channels) < 1 || len(d.Channels) > 2 {
		return Draft{}, segment.Invalid("invalid_channels", "channels")
	}
	seen := map[string]bool{}
	for _, c := range d.Channels {
		if (c != "sms" && c != "email") || seen[c] {
			return Draft{}, segment.Invalid("invalid_channels", "channels")
		}
		seen[c] = true
	}
	d.Channels = nil
	for _, c := range []string{"sms", "email"} {
		if seen[c] {
			d.Channels = append(d.Channels, c)
		}
	}
	return d, nil
}

type Bounds struct {
	Count        int `json:"count"`
	MaxFirstName int `json:"max_first_name_runes"`
	MaxCity      int `json:"max_city_runes"`
}
type Preview struct {
	AsOf           time.Time         `json:"as_of"`
	EligibleUnique int               `json:"eligible_unique"`
	ByChannel      map[string]Bounds `json:"by_channel"`
}
