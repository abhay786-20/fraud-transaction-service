// Package db opens and configures the connection pool to PostgreSQL.
package db

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // registers the "postgres" driver with database/sql; never called directly

	"github.com/abhay786-20/fraud-transaction-service/internal/config"
)

// Connect opens a connection pool to Postgres using cfg, verifies it's
// actually reachable, and returns it ready to use.
func Connect(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name,
	)

	conn, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	conn.SetMaxOpenConns(cfg.MaxOpenConns)
	conn.SetMaxIdleConns(cfg.MaxIdleConns)
	conn.SetConnMaxLifetime(time.Duration(cfg.MaxLifetimeMins) * time.Minute)

	return conn, nil
}
