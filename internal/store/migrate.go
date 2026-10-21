package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"time"

	"github.com/KayaOrhan/SmartCampaignComposer/migrations"
)

func Migrate(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	return migrate(ctx, db, migrations.Files, timeout)
}

// ponytail: one API instance runs migrations; add migration coordination before scaling API replicas.
func migrate(ctx context.Context, db *sql.DB, files fs.FS, timeout time.Duration) (result error) {
	if timeout <= 0 {
		return errors.New("migration timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	entries, err := fs.Glob(files, "*.sql")
	if err != nil {
		return errors.New("list migrations failed")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("begin migrations failed")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			result = errors.Join(result, errors.New("rollback migrations failed"))
		}
	}()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return errors.New("create migration ledger failed")
	}
	for _, name := range entries {
		var applied bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version=$1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s failed", path.Base(name))
		}
		if applied {
			continue
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read migration %s failed", path.Base(name))
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s failed", path.Base(name))
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO public.schema_migrations(version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record migration %s failed", path.Base(name))
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.New("commit migrations failed")
	}
	return nil
}
