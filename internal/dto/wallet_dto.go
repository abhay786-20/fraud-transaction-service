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
	IsEnabled bool      `json:"is_enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewWalletResponse(w *models.Wallet) WalletResponse {
	return WalletResponse{
		UserID:    w.UserID,
		Balance:   w.Balance,
		IsEnabled: w.IsEnabled,
		UpdatedAt: w.UpdatedAt,
	}
}

// WalletStatusResponse is one row of GET /wallets?user_ids=... (admin-only)
// — unlike WalletResponse, Exists can be false: a requested user_id with no
// wallet row at all still gets an entry, so the dashboard's users table can
// render "no wallet" without a separate lookup per row. IsEnabled is a
// pointer, not a plain bool — omitempty on a plain bool would also drop a
// real `false` (a disabled wallet), making it indistinguishable from the
// "wallet doesn't exist" case. A pointer only omits when nil.
type WalletStatusResponse struct {
	UserID    string `json:"user_id"`
	Exists    bool   `json:"exists"`
	IsEnabled *bool  `json:"is_enabled,omitempty"`
	Balance   string `json:"balance,omitempty"`
}

// NewWalletStatusList maps the wallets that were found against the full
// set of requested IDs, filling in Exists: false for any ID with no row.
func NewWalletStatusList(requestedIDs []string, found []models.Wallet) []WalletStatusResponse {
	byUserID := make(map[string]models.Wallet, len(found))
	for _, w := range found {
		byUserID[w.UserID] = w
	}

	statuses := make([]WalletStatusResponse, len(requestedIDs))
	for i, id := range requestedIDs {
		if wallet, ok := byUserID[id]; ok {
			isEnabled := wallet.IsEnabled
			statuses[i] = WalletStatusResponse{UserID: id, Exists: true, IsEnabled: &isEnabled, Balance: wallet.Balance}
		} else {
			statuses[i] = WalletStatusResponse{UserID: id, Exists: false}
		}
	}
	return statuses
}

// SetWalletEnabledRequest is the body for PATCH /wallets/:userId. A
// pointer, not a plain bool — binding:"required" on a plain bool would
// reject `{"is_enabled": false}` (false is bool's zero value, indistinguishable
// from "not sent"), so a pointer is needed to tell "false" apart from "absent".
type SetWalletEnabledRequest struct {
	IsEnabled *bool `json:"is_enabled" binding:"required"`
}
