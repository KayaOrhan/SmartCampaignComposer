package segment

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const validSegment = `{"version":1,"all":[{"field":"city","op":"in","value":["Istanbul"]},{"field":"cart_added","op":"within_days","value":30},{"field":"purchase","op":"none_within_days","value":30}]}`

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"valid", validSegment, true},
		{"unknown version", strings.Replace(validSegment, `"version":1`, `"version":2`, 1), false},
		{"duplicate version", strings.Replace(validSegment, `"version":1`, `"version":2,"version":1`, 1), false},
		{"escaped duplicate", strings.Replace(validSegment, `"version":1`, `"version":2,"vers\u0069on":1`, 1), false},
		{"unknown key", strings.Replace(validSegment, `"version":1`, `"version":1,"sql":"x"`, 1), false},
		{"wrong case", strings.Replace(validSegment, `"field"`, `"FIELD"`, 1), false},
		{"unknown field", strings.Replace(validSegment, `"city"`, `"consent"`, 1), false},
		{"unknown op", strings.Replace(validSegment, `"in"`, `"eq"`, 1), false},
		{"empty list", strings.Replace(validSegment, `["Istanbul"]`, `[]`, 1), false},
		{"null list", strings.Replace(validSegment, `["Istanbul"]`, `null`, 1), false},
		{"null entry", strings.Replace(validSegment, `["Istanbul"]`, `[null]`, 1), false},
		{"wrong type", strings.Replace(validSegment, `["Istanbul"]`, `"Istanbul"`, 1), false},
		{"long city", strings.Replace(validSegment, "Istanbul", strings.Repeat("a", 121), 1), false},
		{"zero days", strings.Replace(validSegment, `:30`, `:0`, 1), false},
		{"large days", strings.Replace(validSegment, `:30`, `:366`, 1), false},
		{"fractional days", strings.Replace(validSegment, `:30`, `:1.5`, 1), false},
		{"one day", strings.ReplaceAll(validSegment, `:30`, `:1`), true},
		{"365 days", strings.ReplaceAll(validSegment, `:30`, `:365`), true},
		{"null", `null`, false},
		{"empty all", `{"version":1,"all":[]}`, false},
		{"missing all", `{"version":1}`, false},
		{"trailing", validSegment + ` {}`, false},
		{"truncated", validSegment[:20], false},
		{"invalid UTF8", string([]byte{0xff}), false},
		{"too large", strings.Repeat(" ", MaxJSONBytes) + validSegment, false},
		{"language", `{"version":1,"all":[{"field":"language","op":"in","value":["TR","en-US"]}]}`, true},
		{"bad language", `{"version":1,"all":[{"field":"language","op":"in","value":["x; sql"]}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.body))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err != nil {
				var v *ValidationError
				if !errors.As(err, &v) {
					t.Fatal("untyped validation error")
				}
			}
		})
	}
	clause := `{"field":"city","op":"in","value":["istanbul"]}`
	for n := 8; n <= 9; n++ {
		_, err := Parse([]byte(`{"version":1,"all":[` + strings.TrimSuffix(strings.Repeat(clause+",", n), ",") + `]}`))
		if (err == nil) != (n == 8) {
			t.Fatal("condition bound")
		}
	}
}

func TestLLMOutputNeverReachesSQL(t *testing.T) {
	marker := "'; DROP TABLE users; --"
	encoded, err := json.Marshal(map[string]any{"version": 1, "all": []any{map[string]any{"field": "city", "op": "in", "value": []string{marker}}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	query, args, err := v.Query("sms", time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(query, "DROP") || !strings.Contains(query, "c.granted=true") {
		t.Fatal("untrusted value reached SQL or consent filter missing")
	}
	if !reflect.DeepEqual(args[1], []string{NormalizeCity(marker)}) {
		t.Fatal("value not bound")
	}
	for _, channel := range []string{"", "push", "sms OR true"} {
		if _, _, err := v.Query(channel, time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)); err == nil {
			t.Fatal("invalid channel accepted")
		}
	}
}

func TestQueryWindowsAndZeroValue(t *testing.T) {
	asOf := time.Date(2026, 10, 21, 12, 0, 0, 0, time.FixedZone("offset", 10800))
	v, err := Parse([]byte(validSegment))
	if err != nil {
		t.Fatal(err)
	}
	query, args, err := v.Query("email", asOf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "NOT EXISTS") || !strings.Contains(query, "e.added_at >= $3 AND e.added_at < $4") {
		t.Fatal("incorrect event bounds")
	}
	if !args[2].(time.Time).Equal(asOf.Add(-30*24*time.Hour)) || args[3].(time.Time).Location() != time.UTC {
		t.Fatal("incorrect UTC window")
	}
	copy := v.JSON()
	copy[0] = 'x'
	if v.JSON()[0] != '{' {
		t.Fatal("validated value mutable")
	}
	if _, _, err := (Validated{}).Query("sms", asOf); err == nil {
		t.Fatal("zero value compiled")
	}
}

func TestDocumentRoundTrip(t *testing.T) {
	values := make([]string, 20)
	for i := range values {
		values[i] = strings.Repeat("<", 120)
	}
	conditions := make([]map[string]any, 8)
	for i := range conditions {
		conditions[i] = map[string]any{"field": "city", "op": "in", "value": values}
	}
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string]any{"version": 1, "all": conditions}); err != nil {
		t.Fatal(err)
	}
	v, err := Parse(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(v.JSON()) <= MaxJSONBytes {
		t.Fatal("fixture does not expand beyond ingress limit")
	}
	if _, err := Parse(v.JSON()); err == nil {
		t.Fatal("HTTP limit was relaxed")
	}
	stored, err := ParseDocument(v.JSON())
	if err != nil || !bytes.Equal(stored.JSON(), v.JSON()) {
		t.Fatal("canonical round trip failed", err)
	}
	for _, body := range []string{
		strings.Repeat(" ", MaxDocumentBytes) + validSegment,
		strings.Replace(validSegment, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(validSegment, `"city"`, `"consent"`, 1),
		strings.Replace(validSegment, `"field"`, `"FIELD"`, 1),
		strings.Replace(validSegment, "Istanbul", strings.Repeat("a", 121), 1),
		validSegment + " {}",
	} {
		if _, err := ParseDocument([]byte(body)); err == nil {
			t.Fatal("invalid stored document accepted")
		}
	}
}
