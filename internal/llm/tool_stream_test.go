package llm

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestToolJSONFragments(t *testing.T) {
	// Generate valid escaping through encoding/json, then force surrogate splits
	// in the synthetic upstream stream as well as normal UTF-8 and quote escapes.
	encoded, err := json.Marshal(map[string]any{"sms": map[string]string{"body": "Hi \" quoted \\ path\n\u0130stanbul \U0001f600 {{first_name}}"}, "email": map[string]string{"subject": "Offer", "body": "{{city}}"}})
	if err != nil {
		t.Fatal(err)
	}
	doc := strings.Replace(string(encoded), "\U0001f600", `\ud83d\ude00`, 1)
	for _, size := range []int{1, 2, 7, len(doc)} {
		d := &toolJSON{}
		actual := map[string]string{}
		calls := 0
		for start := 0; start < len(doc); start += size {
			if err := d.feed(doc[start:min(start+size, len(doc))], func(ch, field, text string) error {
				if !utf8.ValidString(text) || len(text) > 1024 {
					t.Fatal("invalid display chunk")
				}
				actual[ch+"."+field] += text
				calls++
				return nil
			}); err != nil {
				t.Fatalf("size=%d offset=%d err=%v", size, start, err)
			}
		}
		var expected map[string]map[string]string
		if err := json.Unmarshal([]byte(doc), &expected); err != nil {
			t.Fatal(err)
		}
		for ch, fields := range expected {
			for field, text := range fields {
				if actual[ch+"."+field] != text {
					t.Fatalf("size=%d wrong field %s.%s: %q", size, ch, field, actual[ch+"."+field])
				}
			}
		}
		if calls < 3 || string(d.body) != doc {
			t.Fatal("lost document/deltas")
		}
	}
}

func TestToolJSONProvisionalPrefixAndBounds(t *testing.T) {
	d := &toolJSON{}
	text := ""
	emit := func(ch, field, delta string) error { text += delta; return nil }
	if err := d.feed(`{"sms":{"body":"partial`, emit); err != nil || text != "partial" {
		t.Fatal("field buffered until closing quote", err, text)
	}
	if err := d.feed(` text"}}`, emit); err != nil || text != "partial text" {
		t.Fatal("lost continuation", err, text)
	}
	for _, raw := range []string{
		`{"sms":{"body":"one","body":"two"}}`,
		`{"sms":{"body":"\q`,
		`{"sms":{"body":"\udc00`,
		strings.Repeat(" ", 32769),
	} {
		if err := (&toolJSON{}).feed(raw, emit); err == nil {
			t.Fatal("unsafe stream accepted")
		}
	}
	for _, raw := range []string{`{"push":{"body":"not displayed"}}`, `{"sms":{"sql":"not displayed"}}`} {
		if err := (&toolJSON{}).feed(raw, func(string, string, string) error { t.Fatal("unknown field displayed"); return nil }); err != nil {
			t.Fatal(err)
		}
	}
}
