package handler

import (
	"errors"
	"net/http"

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
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewWalletResponse(wallet))
}
