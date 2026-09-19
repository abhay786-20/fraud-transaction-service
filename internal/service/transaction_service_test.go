package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/authclient"
	"github.com/abhay786-20/fraud-transaction-service/internal/models"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
)

// fakeTransactionRepository is an in-memory stand-in for
// TransactionRepository — no real database involved. Direct payoff of
// TransactionService depending on the interface, not the concrete
// Postgres type.
type fakeTransactionRepository struct {
	createErr error
	created   *models.Transaction
}

func (f *fakeTransactionRepository) Create(ctx context.Context, txn *models.Transaction, event *models.OutboxEvent) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = txn
	return nil
}

func (f *fakeTransactionRepository) GetByID(ctx context.Context, id string) (*models.Transaction, error) {
	if f.created != nil && f.created.ID == id {
		return f.created, nil
	}
	return nil, repository.ErrTransactionNotFound
}

func (f *fakeTransactionRepository) List(ctx context.Context, filter repository.TransactionListFilter) ([]models.Transaction, int, error) {
	if f.created == nil {
		return nil, 0, nil
	}
	return []models.Transaction{*f.created}, 1, nil
}

// fakeUserVerifier is an in-memory stand-in for UserVerifier (normally
// *authclient.Client, calling fraud-auth-service over HTTP) — this is
// exactly why UserVerifier was pulled out as an interface in the first
// place: no real network call needed here.
type fakeUserVerifier struct {
	users map[string]*authclient.User
}

func newFakeUserVerifier() *fakeUserVerifier {
	return &fakeUserVerifier{users: make(map[string]*authclient.User)}
}

func (f *fakeUserVerifier) GetUser(ctx context.Context, id string) (*authclient.User, error) {
	user, ok := f.users[id]
	if !ok {
		return nil, authclient.ErrUserNotFound
	}
	return user, nil
}

func TestTransactionService_Create(t *testing.T) {
	const senderID = "sender-1"
	const activeReceiverID = "receiver-active"
	const inactiveReceiverID = "receiver-inactive"

	newVerifier := func() *fakeUserVerifier {
		v := newFakeUserVerifier()
		v.users[activeReceiverID] = &authclient.User{ID: activeReceiverID, IsActive: true}
		v.users[inactiveReceiverID] = &authclient.User{ID: inactiveReceiverID, IsActive: false}
		return v
	}

	tests := []struct {
		name       string
		receiverID string
		amount     string
		repoErr    error
		wantErr    error
	}{
		{
			name:       "successful transaction",
			receiverID: activeReceiverID,
			amount:     "100.00",
		},
		{
			name:       "non-numeric amount rejected",
			receiverID: activeReceiverID,
			amount:     "not-a-number",
			wantErr:    ErrInvalidAmount,
		},
		{
			name:       "zero amount rejected",
			receiverID: activeReceiverID,
			amount:     "0",
			wantErr:    ErrInvalidAmount,
		},
		{
			name:       "receiver not found",
			receiverID: "nobody",
			amount:     "100.00",
			wantErr:    ErrReceiverNotFound,
		},
		{
			name:       "receiver inactive",
			receiverID: inactiveReceiverID,
			amount:     "100.00",
			wantErr:    ErrReceiverInactive,
		},
		{
			name:       "repository error (e.g. insufficient balance) bubbles up unchanged",
			receiverID: activeReceiverID,
			amount:     "100.00",
			repoErr:    repository.ErrInsufficientBalance,
			wantErr:    repository.ErrInsufficientBalance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeTransactionRepository{createErr: tt.repoErr}
			svc := NewTransactionService(repo, newVerifier(), zap.NewNop())

			txn, err := svc.Create(context.Background(), senderID, tt.receiverID, tt.amount, "")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if txn.SenderID != senderID {
				t.Errorf("got sender %q, want %q", txn.SenderID, senderID)
			}
			if txn.ReceiverID != tt.receiverID {
				t.Errorf("got receiver %q, want %q", txn.ReceiverID, tt.receiverID)
			}
			if txn.Currency != "INR" {
				t.Errorf("got currency %q, want default %q", txn.Currency, "INR")
			}
			if txn.ID == "" {
				t.Error("expected a generated transaction ID")
			}
		})
	}
}
