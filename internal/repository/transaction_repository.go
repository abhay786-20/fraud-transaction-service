package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

var (
	ErrTransactionNotFound = errors.New("transaction not found")
	// ErrTransactionNotFlaggable covers every status Flag refuses to
	// transition from: already 'flagged' (no double-notifying), or
	// 'pending'/'failed' (nothing to flag as suspicious yet/anymore). One
	// sentinel for all three — the handler doesn't need to tell them apart,
	// the admin just needs to know this transaction can't be flagged right now.
	ErrTransactionNotFlaggable = errors.New("transaction cannot be flagged from its current status")
	// ErrTransactionNotFlagged is Unflag's mirror of ErrTransactionNotFlaggable
	// — returned when the transaction isn't currently 'flagged', so there's
	// nothing to clear.
	ErrTransactionNotFlagged = errors.New("transaction is not currently flagged")
)

// TransactionListFilter powers the admin dashboard's Transactions tab
// search/filter bar. This service owns none of the sender/receiver
// name/email data being searched, so Search/SearchUserIDs are two halves
// of ONE logical search box the caller already split: the dashboard
// resolves a typed name to user IDs via fraud-auth-service first, then
// hands both the raw text (matched against this service's own id column)
// and the resolved IDs (matched against sender_id/receiver_id) here —
// see the OR'd search clause in buildTransactionFilter.
type TransactionListFilter struct {
	// Status, when non-empty, is an exact match against the status column.
	Status string
	// Search, when non-empty, matches transactions whose ID contains this
	// text (case-insensitive).
	Search string
	// SearchUserIDs, when non-empty, matches transactions whose sender OR
	// receiver is one of these IDs.
	SearchUserIDs []string
	Limit         int
	Offset        int
}

type TransactionRepository interface {
	// Create atomically inserts both txn and event in a single database
	// transaction — the Outbox Pattern's actual guarantee: the event can
	// never be committed without the transaction row, or vice versa.
	Create(ctx context.Context, txn *models.Transaction, event *models.OutboxEvent) error
	GetByID(ctx context.Context, id string) (*models.Transaction, error)
	// List powers the admin dashboard's Transactions tab — most-recent
	// first, paginated, plus the total count across all pages.
	List(ctx context.Context, filter TransactionListFilter) ([]models.Transaction, int, error)
	// Flag transitions a transaction from 'completed' to 'flagged' — an
	// admin's manual call that a completed transaction looks fraudulent,
	// distinct from fraud-engine-service's automatic scoring. Only
	// 'completed' transactions are eligible; see ErrTransactionNotFlaggable.
	Flag(ctx context.Context, id string) (*models.Transaction, error)
	// Unflag is Flag's reverse — an admin's manual call that a flagged
	// transaction was a false alarm. Only 'flagged' transactions are
	// eligible; see ErrTransactionNotFlagged.
	Unflag(ctx context.Context, id string) (*models.Transaction, error)
}

type postgresTransactionRepository struct {
	db         *sqlx.DB
	walletRepo walletTransactor
	log        *zap.Logger
}

func NewTransactionRepository(db *sqlx.DB, walletRepo walletTransactor, log *zap.Logger) TransactionRepository {
	return &postgresTransactionRepository{db: db, walletRepo: walletRepo, log: log}
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

	// Debit sender, credit receiver — using tx, the SAME in-flight
	// transaction as the inserts below, via WalletRepository's
	// dbExecutor-typed methods. If the sender's balance is too low,
	// Debit fails here and NOTHING commits: no money moves, no
	// transaction row, no outbox event — the deferred Rollback cleans up
	// the whole attempt atomically.
	if err := r.walletRepo.Debit(ctx, tx, txn.SenderID, txn.Amount); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("sender wallet: %w", err)
	}
	if err := r.walletRepo.Credit(ctx, tx, txn.ReceiverID, txn.Amount); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("receiver wallet: %w", err)
	}

	// txn.ID and event.AggregateID are already set by the caller (service
	// layer generates the ID in Go, before this call) — specifically so
	// the outbox event's payload can already contain the transaction ID
	// when it's built, with no chicken-and-egg ordering problem.
	const insertTxn = `
		INSERT INTO transactions (id, sender_id, receiver_id, amount, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at`

	row := tx.QueryRowxContext(ctx, insertTxn,
		txn.ID, txn.SenderID, txn.ReceiverID, txn.Amount, txn.Currency, txn.Status,
	)
	if err := row.Scan(&txn.CreatedAt, &txn.UpdatedAt); err != nil {
		r.debugQuery("transaction.Create", start, err)
		return fmt.Errorf("inserting transaction: %w", err)
	}

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

func (r *postgresTransactionRepository) List(ctx context.Context, filter TransactionListFilter) ([]models.Transaction, int, error) {
	start := time.Now()
	where, args := buildTransactionFilter(filter)

	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM transactions " + where
	if err := r.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		r.debugQuery("transaction.List.count", start, err)
		return nil, 0, fmt.Errorf("counting transactions: %w", err)
	}

	listArgs := append(args, limit, offset)
	listQuery := fmt.Sprintf(
		"SELECT * FROM transactions %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, len(args)+1, len(args)+2,
	)

	var txns []models.Transaction
	err := r.db.SelectContext(ctx, &txns, listQuery, listArgs...)
	r.debugQuery("transaction.List", start, err)
	if err != nil {
		return nil, 0, fmt.Errorf("listing transactions: %w", err)
	}

	return txns, total, nil
}

// buildTransactionFilter turns a TransactionListFilter into a "WHERE ..."
// clause plus its matching placeholder arguments — same pattern as
// fraud-auth-service's buildUserFilter. Values always go through numbered
// placeholders, never string-concatenated, so this stays safe from SQL
// injection regardless of filter content.
func buildTransactionFilter(filter TransactionListFilter) (string, []any) {
	var conditions []string
	var args []any

	if filter.Status != "" {
		args = append(args, filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}

	// Search and SearchUserIDs are OR'd together, not AND'd — they're two
	// halves of ONE search box (id text match vs. resolved sender/receiver
	// IDs), not two independent filters. Status stays a separate AND.
	if filter.Search != "" || len(filter.SearchUserIDs) > 0 {
		var searchConditions []string
		if filter.Search != "" {
			args = append(args, "%"+filter.Search+"%")
			searchConditions = append(searchConditions, fmt.Sprintf("id::text ILIKE $%d", len(args)))
		}
		if len(filter.SearchUserIDs) > 0 {
			args = append(args, pq.Array(filter.SearchUserIDs))
			searchConditions = append(searchConditions, fmt.Sprintf("(sender_id = ANY($%d) OR receiver_id = ANY($%d))", len(args), len(args)))
		}
		conditions = append(conditions, "("+strings.Join(searchConditions, " OR ")+")")
	}

	if len(conditions) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}

func (r *postgresTransactionRepository) Flag(ctx context.Context, id string) (*models.Transaction, error) {
	return r.transitionStatus(ctx, id, "transaction.Flag", "completed", "flagged", ErrTransactionNotFlaggable)
}

func (r *postgresTransactionRepository) Unflag(ctx context.Context, id string) (*models.Transaction, error) {
	return r.transitionStatus(ctx, id, "transaction.Unflag", "flagged", "completed", ErrTransactionNotFlagged)
}

// transitionStatus is the shared implementation behind Flag/Unflag — both
// are "move from exactly one required status to another, or tell the
// caller precisely why not" with nothing else different between them.
func (r *postgresTransactionRepository) transitionStatus(
	ctx context.Context, id, op, fromStatus, toStatus string, ineligibleErr error,
) (*models.Transaction, error) {
	start := time.Now()
	const query = `
		UPDATE transactions SET status = $2, updated_at = now()
		WHERE id = $1 AND status = $3
		RETURNING sender_id, receiver_id, amount, currency, status, created_at, updated_at`

	txn := &models.Transaction{ID: id}
	row := r.db.QueryRowxContext(ctx, query, id, toStatus, fromStatus)
	err := row.Scan(&txn.SenderID, &txn.ReceiverID, &txn.Amount, &txn.Currency, &txn.Status, &txn.CreatedAt, &txn.UpdatedAt)
	r.debugQuery(op, start, err)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Zero rows: either the transaction doesn't exist at all, or it
			// exists but wasn't in fromStatus — distinguish so the admin
			// gets an accurate error either way.
			var exists bool
			if lookupErr := r.db.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM transactions WHERE id = $1)", id); lookupErr != nil {
				return nil, fmt.Errorf("checking transaction existence: %w", lookupErr)
			}
			if !exists {
				return nil, ErrTransactionNotFound
			}
			return nil, ineligibleErr
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return txn, nil
}
