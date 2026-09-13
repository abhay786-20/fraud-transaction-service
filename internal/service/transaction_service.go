// Package service holds business logic. It depends on repository
// interfaces, never on database/sql or sqlx directly.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/abhay786-20/fraud-transaction-service/internal/authclient"
	"github.com/abhay786-20/fraud-transaction-service/internal/models"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
)

var (
	ErrInvalidAmount    = errors.New("amount must be a positive number")
	ErrReceiverNotFound = errors.New("receiver account not found")
	ErrReceiverInactive = errors.New("receiver account is inactive")
)

type TransactionService interface {
	Create(ctx context.Context, senderID, receiverID, amount, currency string) (*models.Transaction, error)
}

type transactionService struct {
	txnRepo    repository.TransactionRepository
	authClient *authclient.Client
	log        *zap.Logger
}

func NewTransactionService(txnRepo repository.TransactionRepository, authClient *authclient.Client, log *zap.Logger) TransactionService {
	return &transactionService{txnRepo: txnRepo, authClient: authClient, log: log}
}

// transactionCreatedPayload is the JSON shape published to Kafka once the
// outbox worker picks this event up. Deliberately its own type — decoupled
// from models.Transaction, so the wire event's shape can evolve
// independently of the database schema.
type transactionCreatedPayload struct {
	TransactionID string `json:"transaction_id"`
	SenderID      string `json:"sender_id"`
	ReceiverID    string `json:"receiver_id"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
}

func (s *transactionService) Create(ctx context.Context, senderID, receiverID, amount, currency string) (*models.Transaction, error) {
	parsedAmount, err := strconv.ParseFloat(amount, 64)
	if err != nil || parsedAmount <= 0 {
		return nil, ErrInvalidAmount
	}

	if currency == "" {
		currency = "INR"
	}

	// Verify the receiver is real BEFORE ever touching the database —
	// unlike sender_id (proven by the caller's JWT), nothing else here
	// vouches for receiverID at all.
	receiver, err := s.authClient.GetUser(ctx, receiverID)
	if err != nil {
		if errors.Is(err, authclient.ErrUserNotFound) {
			s.log.Warn("transaction rejected: receiver not found",
				zap.String("sender_id", senderID), zap.String("receiver_id", receiverID))
			return nil, ErrReceiverNotFound
		}
		s.log.Error("verifying receiver failed",
			zap.String("sender_id", senderID), zap.String("receiver_id", receiverID), zap.Error(err))
		return nil, fmt.Errorf("verifying receiver: %w", err)
	}
	if !receiver.IsActive {
		s.log.Warn("transaction rejected: receiver inactive",
			zap.String("sender_id", senderID), zap.String("receiver_id", receiverID))
		return nil, ErrReceiverInactive
	}

	// Generated here, in Go, BEFORE any database call — not left to the
	// database's DEFAULT gen_random_uuid(). That's what lets the outbox
	// payload below already know the transaction's ID, with no ordering
	// problem: nothing here waits on a database round-trip to happen first.
	txnID := uuid.NewString()

	txn := &models.Transaction{
		ID:         txnID,
		SenderID:   senderID,
		ReceiverID: receiverID,
		Amount:     amount, // the ORIGINAL string — never the parsed float, no precision lost
		Currency:   currency,
		Status:     "completed",
	}

	payload, err := json.Marshal(transactionCreatedPayload{
		TransactionID: txnID,
		SenderID:      senderID,
		ReceiverID:    receiverID,
		Amount:        amount,
		Currency:      currency,
		Status:        txn.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling event payload: %w", err)
	}

	event := &models.OutboxEvent{
		AggregateType: "transaction",
		AggregateID:   txnID,
		EventType:     "transaction.created",
		Payload:       payload,
	}

	if err := s.txnRepo.Create(ctx, txn, event); err != nil {
		s.log.Error("creating transaction failed",
			zap.String("sender_id", senderID), zap.String("receiver_id", receiverID), zap.Error(err))
		return nil, err
	}

	s.log.Info("transaction created",
		zap.String("transaction_id", txn.ID),
		zap.String("sender_id", senderID),
		zap.String("receiver_id", receiverID),
		zap.String("amount", amount),
	)
	return txn, nil
}
