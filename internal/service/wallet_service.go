package service

import (
	"context"
	"errors"
	"strconv"

	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
)

type WalletService interface {
	Create(ctx context.Context, userID string) (*models.Wallet, error)
	GetMine(ctx context.Context, userID string) (*models.Wallet, error)
	TopUp(ctx context.Context, userID, amount string) (*models.Wallet, error)
}

type walletService struct {
	walletRepo repository.WalletRepository
	log        *zap.Logger
}

func NewWalletService(walletRepo repository.WalletRepository, log *zap.Logger) WalletService {
	return &walletService{walletRepo: walletRepo, log: log}
}

func (s *walletService) Create(ctx context.Context, userID string) (*models.Wallet, error) {
	wallet, err := s.walletRepo.Create(ctx, userID)
	if err != nil {
		if !errors.Is(err, repository.ErrWalletAlreadyExists) {
			s.log.Error("creating wallet failed", zap.String("user_id", userID), zap.Error(err))
		}
		return nil, err
	}

	s.log.Info("wallet created", zap.String("user_id", userID))
	return wallet, nil
}

func (s *walletService) GetMine(ctx context.Context, userID string) (*models.Wallet, error) {
	return s.walletRepo.GetByUserID(ctx, userID)
}

func (s *walletService) TopUp(ctx context.Context, userID, amount string) (*models.Wallet, error) {
	parsedAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil || parsedAmount <= 0 {
		return nil, ErrInvalidAmount // same sentinel transaction creation uses — same package, same rule
	}

	wallet, err := s.walletRepo.AddBalance(ctx, userID, amount)
	if err != nil {
		if !errors.Is(err, repository.ErrWalletNotFound) {
			s.log.Error("topping up wallet failed", zap.String("user_id", userID), zap.Error(err))
		}
		return nil, err
	}

	s.log.Info("wallet topped up", zap.String("user_id", userID), zap.String("amount", amount))
	return wallet, nil
}
