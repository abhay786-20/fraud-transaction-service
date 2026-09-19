package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/abhay786-20/fraud-transaction-service/internal/dto"
	"github.com/abhay786-20/fraud-transaction-service/internal/middleware"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
	"github.com/abhay786-20/fraud-transaction-service/internal/service"
)

type WalletHandler struct {
	walletService service.WalletService
}

func NewWalletHandler(walletService service.WalletService) *WalletHandler {
	return &WalletHandler{walletService: walletService}
}

// Create handles POST /wallets — always for the caller's own account.
func (h *WalletHandler) Create(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized"})
		return
	}

	wallet, err := h.walletService.Create(c.Request.Context(), claims.UserID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrWalletAlreadyExists):
			c.JSON(http.StatusConflict, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusCreated, dto.NewWalletResponse(wallet))
}

// GetMine handles GET /wallets/me.
func (h *WalletHandler) GetMine(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized"})
		return
	}

	wallet, err := h.walletService.GetMine(c.Request.Context(), claims.UserID)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrWalletNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewWalletResponse(wallet))
}

// TopUp handles POST /wallets/topup.
func (h *WalletHandler) TopUp(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req dto.TopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	wallet, err := h.walletService.TopUp(c.Request.Context(), claims.UserID, req.Amount)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAmount):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrWalletNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "wallet not found — create one first"})
		case errors.Is(err, repository.ErrWalletDisabled):
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewWalletResponse(wallet))
}

// List handles GET /wallets?user_ids=a,b,c — admin-only, enforced by
// middleware.RequireAdmin on the route. Powers the admin dashboard's users
// table: one call per page instead of one GET /wallets/:id per row.
func (h *WalletHandler) List(c *gin.Context) {
	raw := c.Query("user_ids")
	if raw == "" {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "user_ids query param is required"})
		return
	}
	userIDs := strings.Split(raw, ",")

	wallets, err := h.walletService.ListByUserIDs(c.Request.Context(), userIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"wallets": dto.NewWalletStatusList(userIDs, wallets)})
}

// SetEnabled handles PATCH /wallets/:userId — admin-only, enforced by
// middleware.RequireAdmin on the route. Disabling freezes the wallet:
// Debit/Credit/AddBalance all reject it until re-enabled.
func (h *WalletHandler) SetEnabled(c *gin.Context) {
	var req dto.SetWalletEnabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	userID := c.Param("userId")
	wallet, err := h.walletService.SetEnabled(c.Request.Context(), userID, *req.IsEnabled)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrWalletNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewWalletResponse(wallet))
}
