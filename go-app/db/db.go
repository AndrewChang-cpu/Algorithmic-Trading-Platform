package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Pool *pgxpool.Pool

func Init(databaseURL string) error {
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		return fmt.Errorf("pinging database: %w", err)
	}
	Pool = pool
	return nil
}
