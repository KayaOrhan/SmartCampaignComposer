package main

import (
	"strings"
	"testing"
)

func TestInspect(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
		fail bool
	}{
		{"safe.md", []byte("safe documentation\n"), false},
		{"secret.md", []byte("sk-ant-" + strings.Repeat("x", 12)), true},
		{"hidden.md", []byte("hidden" + string(rune(0x202e))), true},
		{"bad.md", []byte{0xff}, true},
		{"go.mod", []byte("require github.com/twilio/twilio-go v1.0.0"), true},
		{"bad.go", []byte("package bad\nfunc f(){ }"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := inspect(tc.name, tc.body); (err != nil) != tc.fail {
				t.Fatalf("error=%v want failure=%v", err, tc.fail)
			}
		})
	}
}

func TestProtectedPaths(t *testing.T) {
	for _, name := range []string{".env", ".ENV.local", "key.pem", "id_rsa", "id_ed25519"} {
		if !protected(name) {
			t.Errorf("not protected: %s", name)
		}
	}
	for _, name := range []string{".env.example", ".env.sample", ".env.template", "README.md"} {
		if protected(name) {
			t.Errorf("unexpected protection: %s", name)
		}
	}
}
