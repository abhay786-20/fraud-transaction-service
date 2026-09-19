package dto

import (
	"time"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

// CreateTransactionRequest is the expected JSON body for POST /transactions.
// Notice: no sender_id field. It's structurally impossible to set it from
// the request — it always comes from the caller's validated JWT (see the
// handler), never from client input.
type CreateTransactionRequest struct {
	ReceiverID string `json:"receiver_id" binding:"required,uuid"`
	// Amount is a JSON STRING ("500.00"), not a number — avoids
	// floating-point parsing on the wire for money, same reasoning as
	// NUMERIC in the database.
	Amount   string `json:"amount" binding:"required,numeric"`
	Currency string `json:"currency,omitempty" binding:"omitempty,len=3"`
}

// TransactionResponse is the public shape of a transaction.
type TransactionResponse struct {
	ID         string    `json:"id"`
	SenderID   string    `json:"sender_id"`
	ReceiverID string    `json:"receiver_id"`
	Amount     string    `json:"amount"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewTransactionResponse(t *models.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:         t.ID,
		SenderID:   t.SenderID,
		ReceiverID: t.ReceiverID,
		Amount:     t.Amount,
		Currency:   t.Currency,
		Status:     t.Status,
		CreatedAt:  t.CreatedAt,
	}
}

// TransactionListResponse is the paginated response body for GET /transactions.
type TransactionListResponse struct {
	Transactions []TransactionResponse `json:"transactions"`
	Total        int                   `json:"total"`
	Limit        int                   `json:"limit"`
	Offset       int                   `json:"offset"`
}

func NewTransactionListResponse(txns []models.Transaction, total, limit, offset int) TransactionListResponse {
	responses := make([]TransactionResponse, len(txns))
	for i := range txns {
		responses[i] = NewTransactionResponse(&txns[i])
	}
	return TransactionListResponse{
		Transactions: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}
}
