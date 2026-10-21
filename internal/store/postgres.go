package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, databaseURL string, timeout time.Duration) (*sql.DB, error) {
	if timeout <= 0 {
		return nil, errors.New("database timeout must be positive")
	}
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("parse database configuration failed")
	}
	cfg.ConnectTimeout = timeout
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		closeErr := db.Close()
		if closeErr != nil {
			return nil, errors.New("connect to database failed; closing pool also failed")
		}
		// Driver errors can contain credentials or connection strings.
		return nil, errors.New("connect to database failed")
	}
	return db, nil
}
