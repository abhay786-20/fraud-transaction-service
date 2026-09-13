package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
)

// fakeWalletRepository is an in-memory stand-in for WalletRepository —
// only possible to write from this package specifically BECAUSE
// WalletRepository's method set no longer includes Debit/Credit (those
// moved to the unexported walletTransactor interface in package
// repository, which only TransactionRepository ever needs — see the
// comments in wallet_repository.go for why that split had to happen).
type fakeWalletRepository struct {
	wallets map[string]*models.Wallet
}

func newFakeWalletRepository() *fakeWalletRepository {
	return &fakeWalletRepository{wallets: make(map[string]*models.Wallet)}
}

func (f *fakeWalletRepository) Create(ctx context.Context, userID string) (*models.Wallet, error) {
	if _, exists := f.wallets[userID]; exists {
		return nil, repository.ErrWalletAlreadyExists
	}
	wallet := &models.Wallet{UserID: userID, Balance: "0.00"}
	f.wallets[userID] = wallet
	return wallet, nil
}

func (f *fakeWalletRepository) GetByUserID(ctx context.Context, userID string) (*models.Wallet, error) {
	wallet, ok := f.wallets[userID]
	if !ok {
		return nil, repository.ErrWalletNotFound
	}
	return wallet, nil
}

func (f *fakeWalletRepository) AddBalance(ctx context.Context, userID, amount string) (*models.Wallet, error) {
	wallet, ok := f.wallets[userID]
	if !ok {
		return nil, repository.ErrWalletNotFound
	}
	wallet.Balance = amount // simplified — real arithmetic isn't what this test is checking
	return wallet, nil
}

func TestWalletService_Create(t *testing.T) {
	repo := newFakeWalletRepository()
	svc := NewWalletService(repo, zap.NewNop())
	ctx := context.Background()

	wallet, err := svc.Create(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wallet.UserID != "user-1" {
		t.Errorf("got user_id %q, want %q", wallet.UserID, "user-1")
	}

	if _, err := svc.Create(ctx, "user-1"); !errors.Is(err, repository.ErrWalletAlreadyExists) {
		t.Fatalf("got error %v, want %v", err, repository.ErrWalletAlreadyExists)
	}
}

func TestWalletService_TopUp(t *testing.T) {
	tests := []struct {
		name       string
		seedWallet bool
		amount     string
		wantErr    error
	}{
		{
			name:       "successful top up",
			seedWallet: true,
			amount:     "500.00",
		},
		{
			name:       "non-numeric amount rejected",
			seedWallet: true,
			amount:     "not-a-number",
			wantErr:    ErrInvalidAmount,
		},
		{
			name:       "negative amount rejected",
			seedWallet: true,
			amount:     "-10",
			wantErr:    ErrInvalidAmount,
		},
		{
			name:       "no wallet yet",
			seedWallet: false,
			amount:     "500.00",
			wantErr:    repository.ErrWalletNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeWalletRepository()
			svc := NewWalletService(repo, zap.NewNop())
			ctx := context.Background()

			if tt.seedWallet {
				if _, err := svc.Create(ctx, "user-1"); err != nil {
					t.Fatalf("seeding wallet failed: %v", err)
				}
			}

			wallet, err := svc.TopUp(ctx, "user-1", tt.amount)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wallet.Balance != tt.amount {
				t.Errorf("got balance %q, want %q", wallet.Balance, tt.amount)
			}
		})
	}
}

func TestWalletService_GetMine(t *testing.T) {
	repo := newFakeWalletRepository()
	svc := NewWalletService(repo, zap.NewNop())
	ctx := context.Background()

	if _, err := svc.GetMine(ctx, "nonexistent"); !errors.Is(err, repository.ErrWalletNotFound) {
		t.Fatalf("got error %v, want %v", err, repository.ErrWalletNotFound)
	}

	if _, err := svc.Create(ctx, "user-1"); err != nil {
		t.Fatalf("creating wallet failed: %v", err)
	}

	wallet, err := svc.GetMine(ctx, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wallet.UserID != "user-1" {
		t.Errorf("got user_id %q, want %q", wallet.UserID, "user-1")
	}
}
