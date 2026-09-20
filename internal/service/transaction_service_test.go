package service

import (
	"context"
	"encoding/json"
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

func (f *fakeTransactionRepository) Flag(ctx context.Context, id string) (*models.Transaction, error) {
	if f.created == nil || f.created.ID != id {
		return nil, repository.ErrTransactionNotFound
	}
	if f.created.Status != "completed" {
		return nil, repository.ErrTransactionNotFlaggable
	}
	f.created.Status = "flagged"
	return f.created, nil
}

func (f *fakeTransactionRepository) Unflag(ctx context.Context, id string) (*models.Transaction, error) {
	if f.created == nil || f.created.ID != id {
		return nil, repository.ErrTransactionNotFound
	}
	if f.created.Status != "flagged" {
		return nil, repository.ErrTransactionNotFlagged
	}
	f.created.Status = "completed"
	return f.created, nil
}

// fakeAlertPublisher is an in-memory stand-in for AlertPublisher (normally
// *kafka.Producer) — records published messages instead of touching a
// real Kafka broker.
type fakeAlertPublisher struct {
	published  [][]byte
	publishErr error
}

func (f *fakeAlertPublisher) Publish(ctx context.Context, key string, value []byte) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	f.published = append(f.published, value)
	return nil
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
			svc := NewTransactionService(repo, newVerifier(), &fakeAlertPublisher{}, zap.NewNop())

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

func TestTransactionService_Flag(t *testing.T) {
	t.Run("flags a completed transaction and publishes an alert", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{
			ID: "txn-1", SenderID: "sender-1", ReceiverID: "receiver-1", Amount: "500.00", Currency: "INR", Status: "completed",
		}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		txn, err := svc.Flag(context.Background(), "txn-1", "looks like a mule account")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if txn.Status != "flagged" {
			t.Errorf("got status %q, want %q", txn.Status, "flagged")
		}
		if len(publisher.published) != 1 {
			t.Fatalf("got %d published alerts, want 1", len(publisher.published))
		}

		var alert manualFraudAlertPayload
		if err := json.Unmarshal(publisher.published[0], &alert); err != nil {
			t.Fatalf("published alert isn't valid JSON: %v", err)
		}
		if alert.TransactionID != "txn-1" || alert.SenderID != "sender-1" || alert.ReceiverID != "receiver-1" {
			t.Errorf("published alert has wrong transaction/sender/receiver: %+v", alert)
		}
		if len(alert.TriggeredRules) != 1 || alert.TriggeredRules[0].Reason != "looks like a mule account" {
			t.Errorf("published alert missing the given reason: %+v", alert.TriggeredRules)
		}
	})

	t.Run("empty reason falls back to a default", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "completed"}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Flag(context.Background(), "txn-1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var alert manualFraudAlertPayload
		if err := json.Unmarshal(publisher.published[0], &alert); err != nil {
			t.Fatalf("published alert isn't valid JSON: %v", err)
		}
		if alert.TriggeredRules[0].Reason != defaultFlagReason {
			t.Errorf("got reason %q, want default %q", alert.TriggeredRules[0].Reason, defaultFlagReason)
		}
	})

	t.Run("not found and not-flaggable errors bubble up without publishing", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "flagged"}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Flag(context.Background(), "txn-1", ""); !errors.Is(err, repository.ErrTransactionNotFlaggable) {
			t.Fatalf("got error %v, want %v", err, repository.ErrTransactionNotFlaggable)
		}
		if _, err := svc.Flag(context.Background(), "does-not-exist", ""); !errors.Is(err, repository.ErrTransactionNotFound) {
			t.Fatalf("got error %v, want %v", err, repository.ErrTransactionNotFound)
		}
		if len(publisher.published) != 0 {
			t.Errorf("expected no alerts published, got %d", len(publisher.published))
		}
	})

	t.Run("a publish failure surfaces as an error, even though the flag already committed", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "completed"}}
		publisher := &fakeAlertPublisher{publishErr: errors.New("kafka is down")}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Flag(context.Background(), "txn-1", ""); err == nil {
			t.Fatal("expected an error when publishing fails")
		}
		if repo.created.Status != "flagged" {
			t.Errorf("expected the status change to have already committed, got %q", repo.created.Status)
		}
	})
}

func TestTransactionService_Unflag(t *testing.T) {
	t.Run("unflags a flagged transaction and publishes a cleared alert", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{
			ID: "txn-1", SenderID: "sender-1", ReceiverID: "receiver-1", Amount: "500.00", Currency: "INR", Status: "flagged",
		}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		txn, err := svc.Unflag(context.Background(), "txn-1", "confirmed legitimate with the sender")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if txn.Status != "completed" {
			t.Errorf("got status %q, want %q", txn.Status, "completed")
		}
		if len(publisher.published) != 1 {
			t.Fatalf("got %d published alerts, want 1", len(publisher.published))
		}

		var alert manualFraudAlertPayload
		if err := json.Unmarshal(publisher.published[0], &alert); err != nil {
			t.Fatalf("published alert isn't valid JSON: %v", err)
		}
		if !alert.Cleared {
			t.Error("expected Cleared to be true")
		}
		if alert.Reason != "confirmed legitimate with the sender" {
			t.Errorf("got reason %q, want the given reason", alert.Reason)
		}
		if alert.TransactionID != "txn-1" || alert.SenderID != "sender-1" || alert.ReceiverID != "receiver-1" {
			t.Errorf("published alert has wrong transaction/sender/receiver: %+v", alert)
		}
	})

	t.Run("empty reason falls back to a default", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "flagged"}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Unflag(context.Background(), "txn-1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var alert manualFraudAlertPayload
		if err := json.Unmarshal(publisher.published[0], &alert); err != nil {
			t.Fatalf("published alert isn't valid JSON: %v", err)
		}
		if alert.Reason != defaultUnflagReason {
			t.Errorf("got reason %q, want default %q", alert.Reason, defaultUnflagReason)
		}
	})

	t.Run("not found and not-flagged errors bubble up without publishing", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "completed"}}
		publisher := &fakeAlertPublisher{}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Unflag(context.Background(), "txn-1", ""); !errors.Is(err, repository.ErrTransactionNotFlagged) {
			t.Fatalf("got error %v, want %v", err, repository.ErrTransactionNotFlagged)
		}
		if _, err := svc.Unflag(context.Background(), "does-not-exist", ""); !errors.Is(err, repository.ErrTransactionNotFound) {
			t.Fatalf("got error %v, want %v", err, repository.ErrTransactionNotFound)
		}
		if len(publisher.published) != 0 {
			t.Errorf("expected no alerts published, got %d", len(publisher.published))
		}
	})

	t.Run("a publish failure surfaces as an error, even though the unflag already committed", func(t *testing.T) {
		repo := &fakeTransactionRepository{created: &models.Transaction{ID: "txn-1", Status: "flagged"}}
		publisher := &fakeAlertPublisher{publishErr: errors.New("kafka is down")}
		svc := NewTransactionService(repo, newFakeUserVerifier(), publisher, zap.NewNop())

		if _, err := svc.Unflag(context.Background(), "txn-1", ""); err == nil {
			t.Fatal("expected an error when publishing fails")
		}
		if repo.created.Status != "completed" {
			t.Errorf("expected the status change to have already committed, got %q", repo.created.Status)
		}
	})
}
