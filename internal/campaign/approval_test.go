package campaign

import (
	"strings"
	"testing"
)

func TestApprovalFingerprintValidation(t *testing.T) {
	for _, value := range []string{"", strings.Repeat("a", 63), strings.Repeat("A", 64), strings.Repeat("z", 64), strings.Repeat("0", 65)} {
		if ValidateApprovalFingerprint(value) == nil {
			t.Fatal("invalid fingerprint accepted")
		}
	}
	if err := ValidateApprovalFingerprint(strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestLiteralMessageRendering(t *testing.T) {
	channels := []string{"sms", "email"}
	bounds := map[string]Bounds{"sms": {Count: 1, MaxFirstName: 14, MaxCity: 12}, "email": {Count: 1, MaxFirstName: 14, MaxCity: 12}}
	v, err := ValidateMessages([]byte(`{"sms":{"body":"{{first_name}}: {{city}} / {{first_name}}"},"email":{"subject":"Hi {{first_name}}","body":"Visit {{city}}"}}`), channels, bounds)
	if err != nil {
		t.Fatal(err)
	}
	sms, err := v.Render("sms", "{{city}}", "İstanbul")
	if err != nil || sms.Subject != nil || sms.Body != "{{city}}: İstanbul / {{city}}" {
		t.Fatal("recursive or incorrect rendering")
	}
	email, err := v.Render("email", "Özge", "İstanbul")
	if err != nil || email.Subject == nil || *email.Subject != "Hi Özge" || email.Body != "Visit İstanbul" {
		t.Fatal("email rendering failed")
	}
	if _, err := v.Render("push", "Example", "City"); err == nil {
		t.Fatal("unknown channel rendered")
	}
	if _, err := (ValidatedMessages{}).Render("sms", "Example", "City"); err == nil {
		t.Fatal("zero validated value rendered")
	}
}
