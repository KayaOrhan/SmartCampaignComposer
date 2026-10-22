//go:build integration

package store

import (
	"context"
	"database/sql"
	"io/fs"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/migrations"
)

// Run only against the disposable Compose test database, never application data.
func testDatabase(t *testing.T) (*sql.DB, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL must name a disposable test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	parsed, err := url.Parse(databaseURL)
	if err != nil || parsed.Path != "/scc_test" {
		t.Fatal("integration tests require database named scc_test")
	}
	parsed.Path = "/postgres"
	admin, err := Open(ctx, parsed.String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := admin.Close(); err != nil {
			t.Error("close admin pool failed")
		}
	}()
	var created bool
	if err := admin.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='scc_test')`).Scan(&created); err != nil {
		t.Fatal("check test database failed")
	}
	if !created {
		if _, err := admin.ExecContext(ctx, `CREATE DATABASE scc_test`); err != nil {
			t.Fatal("create test database failed")
		}
	}
	db, err := Open(ctx, databaseURL, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error("close test database failed")
		}
	})
	var databaseName string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&databaseName); err != nil || databaseName != "scc_test" {
		t.Fatal("refusing to reset non-test database")
	}
	// Reset only the dedicated database checked above. Keep tests repeatable.
	if _, err := db.ExecContext(ctx, `DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS public.schema_migrations; DROP TABLE IF EXISTS public.rollback_probe`); err != nil {
		t.Fatal("reset disposable database failed")
	}
	return db, ctx
}

func TestMigrations(t *testing.T) {
	db, ctx := testDatabase(t)
	initial, err := fs.ReadFile(migrations.Files, "001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, fstest.MapFS{"001_init.sql": {Data: initial}}, 5*time.Second); err != nil {
		t.Fatal("initial schema:", err)
	}
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db, 5*time.Second); err != nil {
		t.Fatal("repeat migration:", err)
	}
	var count int
	files, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&count); err != nil || count != len(files) {
		t.Fatalf("ledger count=%d error=%v", count, err)
	}
	broken := fstest.MapFS{
		"998_probe.sql":  {Data: []byte(`CREATE TABLE public.rollback_probe(id INT)`)},
		"999_broken.sql": {Data: []byte(`THIS IS NOT SQL`)},
	}
	if err := migrate(ctx, db, broken, 5*time.Second); err == nil {
		t.Fatal("broken migration must fail")
	}
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('public.rollback_probe') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("failed migration did not roll back")
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&count); err != nil || count != len(files) {
		t.Fatal("failed migrations were recorded")
	}
}
