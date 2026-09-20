// Package service holds business logic. It depends on repository
// interfaces, never on database/sql or sqlx directly.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

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
	// List is admin-only (enforced by the handler/route) — powers the
	// admin dashboard's Transactions tab.
	List(ctx context.Context, filter repository.TransactionListFilter) ([]models.Transaction, int, error)
	// Flag is admin-only — a human call that a completed transaction looks
	// fraudulent, distinct from fraud-engine-service's automatic scoring.
	Flag(ctx context.Context, id, reason string) (*models.Transaction, error)
	// Unflag is Flag's reverse — a human call that a flagged transaction
	// was a false alarm.
	Unflag(ctx context.Context, id, reason string) (*models.Transaction, error)
}

// UserVerifier is the one method this service needs from authclient.Client
// — defined here, in the CONSUMING package, not authclient itself (the
// same "consumer defines the interface it needs" idiom as
// repository.TransactionRepository). *authclient.Client already satisfies
// this structurally; a test can hand transactionService a fake instead.
type UserVerifier interface {
	GetUser(ctx context.Context, id string) (*authclient.User, error)
}

// AlertPublisher is the one method this service needs from *kafka.Producer
// — same "consumer defines the interface it needs" idiom as UserVerifier.
// Flag publishes straight through this, not via the Outbox Pattern: see
// the doc comment on Flag for why that split is deliberate here.
type AlertPublisher interface {
	Publish(ctx context.Context, key string, value []byte) error
}

type transactionService struct {
	txnRepo       repository.TransactionRepository
	authClient    UserVerifier
	alertProducer AlertPublisher
	log           *zap.Logger
}

func NewTransactionService(txnRepo repository.TransactionRepository, authClient UserVerifier, alertProducer AlertPublisher, log *zap.Logger) TransactionService {
	return &transactionService{txnRepo: txnRepo, authClient: authClient, alertProducer: alertProducer, log: log}
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

func (s *transactionService) List(ctx context.Context, filter repository.TransactionListFilter) ([]models.Transaction, int, error) {
	return s.txnRepo.List(ctx, filter)
}

// ruleResultPayload/manualFraudAlertPayload mirror fraud-engine-service's
// FraudAlertEvent wire shape exactly — this is what lets
// fraud-notification-service handle a manual admin flag identically to an
// automatic one, with no changes on its end at all.
type ruleResultPayload struct {
	RuleName string `json:"rule_name"`
	Score    int    `json:"score"`
	Reason   string `json:"reason"`
}

type manualFraudAlertPayload struct {
	TransactionID  string              `json:"transaction_id"`
	SenderID       string              `json:"sender_id"`
	ReceiverID     string              `json:"receiver_id"`
	Amount         string              `json:"amount"`
	Currency       string              `json:"currency"`
	TotalScore     int                 `json:"total_score"`
	RiskLevel      string              `json:"risk_level"`
	TriggeredRules []ruleResultPayload `json:"triggered_rules"`
	DetectedAt     time.Time           `json:"detected_at"`
	// Cleared and Reason are only ever set by Unflag — fraud-engine-
	// service's automatic alerts and Flag's manual ones never set Cleared,
	// so it's false/absent for those. fraud-notification-service branches
	// on Cleared to send a "this was reviewed and cleared" email instead
	// of a fraud-alert one, using Reason as the explanation.
	Cleared bool   `json:"cleared,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

const defaultFlagReason = "Manually flagged by admin as suspicious"

// Flag lets an admin mark an already-completed transaction as fraudulent
// after the fact — the case fraud-engine-service's automatic scoring
// missed at transaction time, but a human reviewing it later suspects
// anyway. It transitions the transaction to 'flagged' (only eligible from
// 'completed' — see repository.ErrTransactionNotFlaggable), then publishes
// straight to the SAME "fraud-alerts" Kafka topic fraud-engine-service
// publishes to automatically, in the identical payload shape, so
// fraud-notification-service emails sender + receiver + every admin
// exactly like it would for an automatic alert.
//
// This publish is direct, NOT via the Outbox Pattern — deliberately. Flag
// is a rare, human-triggered side action, not part of the atomic
// money-movement path Outbox exists to protect (nothing here debits or
// credits a wallet). The status change above is already durably
// committed by the time this runs; if the Kafka publish fails, that error
// propagates back to the admin (see handler) so they know to retry,
// rather than the flag silently succeeding with no notification ever
// sent.
func (s *transactionService) Flag(ctx context.Context, id, reason string) (*models.Transaction, error) {
	if reason == "" {
		reason = defaultFlagReason
	}

	txn, err := s.txnRepo.Flag(ctx, id)
	if err != nil {
		if !errors.Is(err, repository.ErrTransactionNotFound) && !errors.Is(err, repository.ErrTransactionNotFlaggable) {
			s.log.Error("flagging transaction failed", zap.String("transaction_id", id), zap.Error(err))
		}
		return nil, err
	}

	payload, err := json.Marshal(manualFraudAlertPayload{
		TransactionID:  txn.ID,
		SenderID:       txn.SenderID,
		ReceiverID:     txn.ReceiverID,
		Amount:         txn.Amount,
		Currency:       txn.Currency,
		TotalScore:     100,
		RiskLevel:      "high",
		TriggeredRules: []ruleResultPayload{{RuleName: "manual_admin_flag", Score: 100, Reason: reason}},
		DetectedAt:     time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling fraud alert payload: %w", err)
	}

	if err := s.alertProducer.Publish(ctx, txn.SenderID, payload); err != nil {
		s.log.Error("publishing manual fraud alert failed", zap.String("transaction_id", id), zap.Error(err))
		return nil, fmt.Errorf("publishing fraud alert: %w", err)
	}

	s.log.Info("transaction manually flagged",
		zap.String("transaction_id", id),
		zap.String("reason", reason),
	)
	return txn, nil
}

const defaultUnflagReason = "Reviewed by admin and cleared as not fraudulent"

// Unflag is Flag's reverse — an admin's manual call that a previously
// flagged transaction was a false alarm. It transitions the transaction
// back to 'completed' (only eligible from 'flagged' — see
// repository.ErrTransactionNotFlagged), then publishes to the same
// "fraud-alerts" topic with Cleared: true, so fraud-notification-service
// sends sender/receiver/every admin a "reviewed and cleared" email
// instead of a fraud-alert one. Same direct-publish-not-Outbox reasoning
// as Flag — see its doc comment.
func (s *transactionService) Unflag(ctx context.Context, id, reason string) (*models.Transaction, error) {
	if reason == "" {
		reason = defaultUnflagReason
	}

	txn, err := s.txnRepo.Unflag(ctx, id)
	if err != nil {
		if !errors.Is(err, repository.ErrTransactionNotFound) && !errors.Is(err, repository.ErrTransactionNotFlagged) {
			s.log.Error("unflagging transaction failed", zap.String("transaction_id", id), zap.Error(err))
		}
		return nil, err
	}

	payload, err := json.Marshal(manualFraudAlertPayload{
		TransactionID: txn.ID,
		SenderID:      txn.SenderID,
		ReceiverID:    txn.ReceiverID,
		Amount:        txn.Amount,
		Currency:      txn.Currency,
		Cleared:       true,
		Reason:        reason,
		DetectedAt:    time.Now(),
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling cleared alert payload: %w", err)
	}

	if err := s.alertProducer.Publish(ctx, txn.SenderID, payload); err != nil {
		s.log.Error("publishing cleared alert failed", zap.String("transaction_id", id), zap.Error(err))
		return nil, fmt.Errorf("publishing cleared alert: %w", err)
	}

	s.log.Info("transaction manually unflagged",
		zap.String("transaction_id", id),
		zap.String("reason", reason),
	)
	return txn, nil
}
