package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

var ErrTransactionNotFound = errors.New("transaction not found")

type TransactionRepository interface {
	// Create atomically inserts both txn and event in a single database
	// transaction — the Outbox Pattern's actual guarantee: the event can
	// never be committed without the transaction row, or vice versa.
	Create(ctx context.Context, txn *models.Transaction, event *models.OutboxEvent) error
	GetByID(ctx context.Context, id string) (*models.Transaction, error)
}

type postgresTransactionRepository struct {
	db  *sqlx.DB
	log *zap.Logger
}

func NewTransactionRepository(db *sqlx.DB, log *zap.Logger) TransactionRepository {
	return &postgresTransactionRepository{db: db, log: log}
}

func (r *postgresTransactionRepository) debugQuery(op string, start time.Time, err error) {
	r.log.Debug("repository query",
		zap.String("op", op),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
}

func (r *postgresTransactionRepository) Create(ctx context.Context, txn *models.Transaction, event *models.OutboxEvent) error {
	start := time.Now()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback() // no-op if Commit succeeds; cleans up on any early return

	const insertTxn = `
		INSERT INTO transactions (sender_id, receiver_id, amount, currency, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at`

	row := tx.QueryRowxContext(ctx, insertTxn,
		txn.SenderID, txn.ReceiverID, txn.Amount, txn.Currency, txn.Status,
	)
	if err := row.Scan(&txn.ID, &txn.CreatedAt, &txn.UpdatedAt); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("inserting transaction: %w", err)
	}

	// Now that we know the real transaction ID, stamp it onto the event
	// before writing it — aggregate_id must point at the row just created.
	event.AggregateID = txn.ID

	const insertEvent = `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	row = tx.QueryRowxContext(ctx, insertEvent,
		event.AggregateType, event.AggregateID, event.EventType, event.Payload,
	)
	if err := row.Scan(&event.ID, &event.CreatedAt); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("inserting outbox event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("committing transaction: %w", err)
	}

	r.debugQuery("transaction.Create", start, nil)
	return nil
}

func (r *postgresTransactionRepository) GetByID(ctx context.Context, id string) (*models.Transaction, error) {
	start := time.Now()
	var txn models.Transaction
	err := r.db.GetContext(ctx, &txn, "SELECT * FROM transactions WHERE id = $1", id)
	r.debugQuery("transaction.GetByID", start, err)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTransactionNotFound
		}
		return nil, fmt.Errorf("getting transaction: %w", err)
	}
	return &txn, nil
}
