package seed

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	a, b := Generate(at), Generate(at)
	if len(a) != 5000 || !reflect.DeepEqual(a, b) {
		t.Fatal("seed is not deterministic or population incorrect")
	}
	missing, denied, granted := 0, 0, 0
	for _, u := range a {
		if !strings.HasSuffix(u.Email, "@example.test") || !strings.HasPrefix(u.Phone, "mock-phone-") {
			t.Fatal("unsafe seed contact format")
		}
		if u.SMS == nil {
			missing++
		} else if !*u.SMS {
			denied++
		} else {
			granted++
		}
	}
	if missing == 0 || denied == 0 || granted == 0 {
		t.Fatal("missing consent scenarios")
	}
}
