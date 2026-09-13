package dto

import (
	"time"

	"github.com/abhay786-20/fraud-transaction-service/internal/models"
)

// TopUpRequest is the expected JSON body for POST /wallets/topup. No
// user_id field — same rule as everywhere else, the wallet being topped
// up is always the caller's own, from the JWT.
type TopUpRequest struct {
	Amount string `json:"amount" binding:"required,numeric"`
}

type WalletResponse struct {
	UserID    string    `json:"user_id"`
	Balance   string    `json:"balance"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewWalletResponse(w *models.Wallet) WalletResponse {
	return WalletResponse{
		UserID:    w.UserID,
		Balance:   w.Balance,
		UpdatedAt: w.UpdatedAt,
	}
}
