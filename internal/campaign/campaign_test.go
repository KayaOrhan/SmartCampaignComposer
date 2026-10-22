package campaign

import (
	"reflect"
	"strings"
	"testing"
)

func TestDraftValidation(t *testing.T) {
	for _, d := range []Draft{{"", []string{"sms"}}, {"brief", nil}, {"brief", []string{"sms", "sms"}}, {"brief", []string{"push"}}, {strings.Repeat("ü", 4001), []string{"email"}}} {
		if _, err := d.Validate(); err == nil {
			t.Fatal("invalid draft accepted")
		}
	}
	d, err := (Draft{" test ", []string{"email", "sms"}}).Validate()
	if err != nil || d.Brief != "test" || !reflect.DeepEqual(d.Channels, []string{"sms", "email"}) {
		t.Fatal("canonical draft invalid")
	}
	if _, err := (Draft{strings.Repeat("ü", 4000), []string{"sms"}}).Validate(); err != nil {
		t.Fatal("rune boundary rejected")
	}
}
