package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrWalletAlreadyExists = errors.New("wallet already exists")
	ErrInsufficientBalance = errors.New("insufficient balance")
)

// dbExecutor is satisfied structurally by both *sqlx.DB and *sqlx.Tx —
// both happen to implement these two methods with matching signatures.
// Debit/Credit accept this instead of a concrete type so the SAME query
// code can run standalone (the wallet API, via the plain DB connection)
// or composed into a LARGER atomic transaction (TransactionRepository.
// Create, which passes its own in-flight *sqlx.Tx instead).
type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetContext(ctx context.Context, dest any, query string, args ...any) error
}

// WalletRepository is the public wallet API — what WalletService depends
// on. Kept free of unexported types in its signatures on purpose, so a
// fake implementing it can be written from ANY package (e.g. a test in
// package service) — see walletTransactor below for why that's not just
// pedantry.
type WalletRepository interface {
	Create(ctx context.Context, userID string) (*models.Wallet, error)
	GetByUserID(ctx context.Context, userID string) (*models.Wallet, error)
	AddBalance(ctx context.Context, userID, amount string) (*models.Wallet, error)
}

// walletTransactor is what TransactionRepository depends on internally —
// unexported, and deliberately a SEPARATE interface from WalletRepository
// above, not folded into it. Debit/Credit's dbExecutor parameter is
// itself unexported, so a type implementing this interface could only
// ever be written inside THIS package anyway — no other package could
// spell out the parameter type to satisfy it. Only postgresWalletRepository
// (below) needs to implement it, and only postgresTransactionRepository
// ever calls it — both live here, in package repository, so this is fine.
type walletTransactor interface {
	Debit(ctx context.Context, exec dbExecutor, userID, amount string) error
	Credit(ctx context.Context, exec dbExecutor, userID, amount string) error
}

type postgresWalletRepository struct {
	db  *sqlx.DB
	log *zap.Logger
}

// NewWalletRepository deliberately returns the CONCRETE type, not the
// WalletRepository interface — *postgresWalletRepository satisfies both
// WalletRepository (for NewWalletService) and walletTransactor (for
// NewTransactionRepository) at once, with no explicit cast needed at
// either call site in main.go. Returning the narrower WalletRepository
// interface here would make it impossible to pass the same value to
// NewTransactionRepository, since WalletRepository's method set doesn't
// include Debit/Credit.
func NewWalletRepository(db *sqlx.DB, log *zap.Logger) *postgresWalletRepository {
	return &postgresWalletRepository{db: db, log: log}
}

func (r *postgresWalletRepository) debugQuery(op string, start time.Time, err error) {
	r.log.Debug("repository query",
		zap.String("op", op),
		zap.Duration("duration", time.Since(start)),
		zap.Error(err),
	)
}

func (r *postgresWalletRepository) Create(ctx context.Context, userID string) (*models.Wallet, error) {
	start := time.Now()
	const query = `INSERT INTO wallets (user_id) VALUES ($1) RETURNING balance, created_at, updated_at`

	wallet := &models.Wallet{UserID: userID}
	row := r.db.QueryRowxContext(ctx, query, userID)
	err := row.Scan(&wallet.Balance, &wallet.CreatedAt, &wallet.UpdatedAt)
	r.debugQuery("wallet.Create", start, err)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrWalletAlreadyExists
		}
		return nil, fmt.Errorf("creating wallet: %w", err)
	}
	return wallet, nil
}

func (r *postgresWalletRepository) GetByUserID(ctx context.Context, userID string) (*models.Wallet, error) {
	start := time.Now()
	var wallet models.Wallet
	err := r.db.GetContext(ctx, &wallet, "SELECT * FROM wallets WHERE user_id = $1", userID)
	r.debugQuery("wallet.GetByUserID", start, err)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("getting wallet: %w", err)
	}
	return &wallet, nil
}

func (r *postgresWalletRepository) AddBalance(ctx context.Context, userID, amount string) (*models.Wallet, error) {
	start := time.Now()
	const query = `
		UPDATE wallets SET balance = balance + $1, updated_at = now()
		WHERE user_id = $2
		RETURNING balance, created_at, updated_at`

	wallet := &models.Wallet{UserID: userID}
	row := r.db.QueryRowxContext(ctx, query, amount, userID)
	err := row.Scan(&wallet.Balance, &wallet.CreatedAt, &wallet.UpdatedAt)
	r.debugQuery("wallet.AddBalance", start, err)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, fmt.Errorf("adding balance: %w", err)
	}
	return wallet, nil
}

// Debit is atomic and race-free in a single statement: the WHERE clause
// itself enforces "only if there's enough balance" — there's no separate
// "check, then update" window for a concurrent debit to sneak in between.
func (r *postgresWalletRepository) Debit(ctx context.Context, exec dbExecutor, userID, amount string) error {
	const query = `
		UPDATE wallets SET balance = balance - $1, updated_at = now()
		WHERE user_id = $2 AND balance >= $1`

	result, err := exec.ExecContext(ctx, query, amount, userID)
	if err != nil {
		return fmt.Errorf("debiting wallet: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		// Zero rows changed means either the wallet doesn't exist, or the
		// balance was too low — distinguish with one more lookup so the
		// caller gets an accurate error either way.
		var exists bool
		if err := exec.GetContext(ctx, &exists, "SELECT EXISTS(SELECT 1 FROM wallets WHERE user_id = $1)", userID); err != nil {
			return fmt.Errorf("checking wallet existence: %w", err)
		}
		if !exists {
			return ErrWalletNotFound
		}
		return ErrInsufficientBalance
	}
	return nil
}

func (r *postgresWalletRepository) Credit(ctx context.Context, exec dbExecutor, userID, amount string) error {
	const query = `UPDATE wallets SET balance = balance + $1, updated_at = now() WHERE user_id = $2`

	result, err := exec.ExecContext(ctx, query, amount, userID)
	if err != nil {
		return fmt.Errorf("crediting wallet: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return ErrWalletNotFound
	}
	return nil
}

// isUniqueViolation checks whether err is specifically a Postgres unique-
// constraint violation (error code 23505 — user_id already exists in this
// case). errors.As, unlike errors.Is, extracts a specific error TYPE
// (here, lib/pq's *pq.Error, which carries a .Code field) rather than
// comparing against one fixed sentinel value.
func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
