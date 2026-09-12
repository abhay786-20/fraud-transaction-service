// Package models holds the Go types that mirror this service's database
// tables.
package models

import "time"

// Transaction mirrors a row in the transactions table (see
// migrations/000001_create_transactions_and_outbox.up.sql).
type Transaction struct {
	ID         string    `db:"id"`
	SenderID   string    `db:"sender_id"`
	ReceiverID string    `db:"receiver_id"`
	Amount     string    `db:"amount"` // NUMERIC scans as string — see note in repository
	Currency   string    `db:"currency"`
	Status     string    `db:"status"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}
