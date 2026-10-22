package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	for _, tc := range []struct {
		name, key, value string
		valid            bool
	}{
		{"defaults", "", "", true},
		{"address", "HTTP_ADDR", "127.0.0.1:9000", true},
		{"bad port", "HTTP_ADDR", ":70000", false},
		{"missing port", "HTTP_ADDR", "localhost", false},
		{"missing database", "DATABASE_URL", "", false},
		{"bad scheme", "DATABASE_URL", "https://localhost/app", false},
		{"missing database name", "DATABASE_URL", "postgres://localhost", false},
		{"timeout", "DB_TIMEOUT", "1s", true},
		{"zero timeout", "DB_TIMEOUT", "0s", false},
		{"negative timeout", "DB_TIMEOUT", "-1s", false},
		{"large timeout", "DB_TIMEOUT", "31s", false},
		{"invalid timeout", "DB_TIMEOUT", "invalid", false},
		{"seed enabled", "SEED_DATA", "true", true},
		{"seed disabled", "SEED_DATA", "false", true},
		{"invalid seed flag", "SEED_DATA", "yes", false},
		{"private invalid URL", "DATABASE_URL", "postgres://user:private-value@%/app", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]string{"DATABASE_URL": "postgres://app:app@localhost/app?sslmode=disable"}
			if tc.key != "" {
				values[tc.key] = tc.value
			}
			cfg, err := Load(func(key string) string { return values[key] })
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "private-value") {
				t.Fatal("configuration error leaked credentials")
			}
			if err == nil && (cfg.DBTimeout <= 0 || cfg.DBTimeout > 30*time.Second) {
				t.Fatal("invalid timeout accepted")
			}
			if err == nil && cfg.SeedData != (values["SEED_DATA"] == "true") {
				t.Fatal("seed flag not applied")
			}
		})
	}
}
