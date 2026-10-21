package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	db, err := Open(context.Background(), "postgres://user:private-value@%/app", time.Second)
	if db != nil || err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("invalid config must fail without leaking credentials")
	}
}

func TestOpenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db, err := Open(ctx, "postgres://app:app@127.0.0.1:1/app?sslmode=disable", time.Second)
	if db != nil || err == nil {
		t.Fatal("cancelled connection must fail")
	}
}

func TestInvalidTimeout(t *testing.T) {
	if _, err := Open(context.Background(), "", 0); err == nil {
		t.Fatal("zero connection timeout accepted")
	}
	if err := Migrate(context.Background(), nil, 0); err == nil {
		t.Fatal("zero migration timeout accepted")
	}
}
