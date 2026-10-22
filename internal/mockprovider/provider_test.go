package mockprovider

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KayaOrhan/SmartCampaignComposer/internal/delivery"
)

func TestMockIngressAndLimits(t *testing.T) {
	subject := "Offer"
	for _, m := range []delivery.Payload{
		{Channel: "sms", Destination: "mock-phone-1", Body: strings.Repeat("İ", 160)},
		{Channel: "email", Destination: "fake@example.test", Subject: &subject, Body: "Hello"},
	} {
		if !valid(m) {
			t.Fatal("valid synthetic payload rejected")
		}
	}
	for _, m := range []delivery.Payload{
		{Channel: "sms", Destination: "mock-phone-1", Body: strings.Repeat("İ", 161)},
		{Channel: "sms", Destination: "real destination", Body: "Hello"},
		{Channel: "email", Destination: "fake@example.com", Subject: &subject, Body: "Hello"},
		{Channel: "sms", Destination: "mock-phone-1", Body: "a\x00b"},
	} {
		if valid(m) {
			t.Fatal("unsafe/overlong payload accepted")
		}
	}
	for _, body := range []string{`{"channel":"sms","channel":"email"}`, `{"unknown":true}`, strings.Repeat("x", 16385)} {
		r := httptest.NewRequest("POST", "/messages", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "campaign:1:user:1:sms")
		w := httptest.NewRecorder()
		(Provider{}).Handler().ServeHTTP(w, r)
		if w.Code != 422 && w.Code != 413 {
			t.Fatal("invalid request reached storage", w.Code)
		}
	}
}
