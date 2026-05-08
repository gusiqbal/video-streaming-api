package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}
	return pool, nil
}

func Migrate(pool *pgxpool.Pool) error {
	_, err := pool.Exec(context.Background(), `
		CREATE TABLE IF NOT EXISTS videos (
			id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
			title        VARCHAR(255) NOT NULL,
			filename     VARCHAR(255) NOT NULL,
			status       VARCHAR(50)  NOT NULL DEFAULT 'pending',
			resolutions  TEXT[]       NOT NULL DEFAULT '{}',
			duration     FLOAT        NOT NULL DEFAULT 0,
			created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		)
	`)
	return err
}
