package campaign

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageValidation(t *testing.T) {
	bounds := map[string]Bounds{"sms": {Count: 2, MaxFirstName: 10, MaxCity: 12}, "email": {Count: 2, MaxFirstName: 10, MaxCity: 12}}
	for _, tc := range []struct {
		name, sms, subject, email string
		valid                     bool
	}{
		{"happy", "Hi {{first_name}} from {{city}}", "Hello", "Hi {{city}}", true},
		{"SMS exact", strings.Repeat("x", 138) + "{{first_name}}{{city}}", "Hello", "Body", true},
		{"both maxima", strings.Repeat("x", 139) + "{{first_name}}{{city}}", "Hello", "Body", false},
		{"repeated maxima", strings.Repeat("x", 117) + "{{first_name}}{{city}}{{first_name}}{{city}}", "Hello", "Body", false},
		{"Unicode exact", strings.Repeat("\u015f", 160), "Hello", "Body", true},
		{"Unicode overflow", strings.Repeat("\u015f", 161), "Hello", "Body", false},
		{"subject exact", "Hi", strings.Repeat("x", 58) + "{{first_name}}{{city}}", "Body", true},
		{"subject overflow", "Hi", strings.Repeat("x", 59) + "{{first_name}}{{city}}", "Body", false},
		{"email exact", "Hi", "Hello", strings.Repeat("x", 1978) + "{{first_name}}{{city}}", true},
		{"email overflow", "Hi", "Hello", strings.Repeat("x", 1979) + "{{first_name}}{{city}}", false},
		{"unknown variable", "{{email}}", "Hello", "Body", false},
		{"malformed variable", "{{first_name}", "Hello", "Body", false},
		{"spaces in variable", "{{ first_name }}", "Hello", "Body", false},
		{"single brace", "Hi }", "Hello", "Body", false},
		{"empty SMS", " ", "Hello", "Body", false},
		{"empty subject", "Hi", " ", "Body", false},
		{"empty body", "Hi", "Hello", " ", false},
		{"NUL", "Hi\x00", "Hello", "Body", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(Messages{SMS: &SMS{Body: tc.sms}, Email: &Email{Subject: tc.subject, Body: tc.email}})
			if err != nil {
				t.Fatal(err)
			}
			v, err := ValidateMessages(body, []string{"sms", "email"}, bounds)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if err == nil {
				copy := v.JSON()
				copy[0] = 'x'
				if v.JSON()[0] != '{' {
					t.Fatal("validated messages mutable")
				}
			}
		})
	}
	for _, body := range []string{
		`{"sms":{"body":"Hi","body":"Bye"},"email":{"subject":"Hi","body":"Body"}}`,
		`{"sms":{"body":"Hi","sql":"DROP"},"email":{"subject":"Hi","body":"Body"}}`,
		`{"SMS":{"body":"Hi"},"email":{"subject":"Hi","body":"Body"}}`,
		`{"sms":null,"email":{"subject":"Hi","body":"Body"}}`,
		`{"sms":{"body":1},"email":{"subject":"Hi","body":"Body"}}`,
		`{"sms":{"body":"Hi"}}`, `null`, `{}`, `{} {}`, string([]byte{255}), strings.Repeat(" ", 32769),
	} {
		if _, err := ValidateMessages([]byte(body), []string{"sms", "email"}, bounds); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
	for _, channels := range [][]string{nil, {"push"}, {"sms", "sms"}, {"email"}} {
		if _, err := ValidateMessages([]byte(`{"sms":{"body":"Hi"}}`), channels, bounds); err == nil {
			t.Fatal("invalid channel set accepted")
		}
	}
	if _, err := ValidateMessages([]byte(`{"sms":{"body":"Hi"}}`), []string{"sms"}, bounds); err != nil {
		t.Fatal("single channel", err)
	}
	if _, err := ValidateMessages([]byte(`{"sms":{"body":"Hi"},"email":null}`), []string{"sms"}, bounds); err == nil {
		t.Fatal("unselected null channel accepted")
	}
	if _, err := ValidateMessages([]byte(`{"sms":{"body":"Hi"}}`), []string{"sms"}, nil); err == nil {
		t.Fatal("missing bounds accepted")
	}
	if _, err := ValidateMessages([]byte(`{"sms":{"body":"{{first_name}}{{city}}"}}`), []string{"sms"}, map[string]Bounds{"sms": {}}); err == nil {
		t.Fatal("empty channel bypassed worst-case bound")
	}
}
