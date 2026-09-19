package models

import "time"

// Wallet mirrors a row in the wallets table. Balance is a string for the
// same reason Transaction.Amount is — NUMERIC has no exact native Go
// equivalent, so we keep it as the raw text Postgres/lib/pq already
// return, never converting through float64.
type Wallet struct {
	UserID    string    `db:"user_id"`
	Balance   string    `db:"balance"`
	IsEnabled bool      `db:"is_enabled"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
