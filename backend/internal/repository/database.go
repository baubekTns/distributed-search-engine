package repository

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenDatabase(
	ctx context.Context,
	dsn string,
) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf(
			"create PostgreSQL connection pool: %w",
			err,
		)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()

		return nil, fmt.Errorf(
			"ping PostgreSQL: %w",
			err,
		)
	}

	return pool, nil
}

func RunMigration(
	ctx context.Context,
	pool *pgxpool.Pool,
	path string,
) error {
	schema, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}

	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
}
