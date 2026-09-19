// Package handler is the HTTP layer — parses requests, calls service,
// writes responses.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/abhay786-20/fraud-transaction-service/internal/dto"
	"github.com/abhay786-20/fraud-transaction-service/internal/middleware"
	"github.com/abhay786-20/fraud-transaction-service/internal/repository"
	"github.com/abhay786-20/fraud-transaction-service/internal/service"
)

type TransactionHandler struct {
	txnService service.TransactionService
}

func NewTransactionHandler(txnService service.TransactionService) *TransactionHandler {
	return &TransactionHandler{txnService: txnService}
}

func (h *TransactionHandler) Create(c *gin.Context) {
	claims := middleware.GetClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "unauthorized"})
		return
	}

	var req dto.CreateTransactionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// sender_id is ALWAYS claims.UserID — never anything from req. There is
	// no field on CreateTransactionRequest that could override this.
	txn, err := h.txnService.Create(c.Request.Context(), claims.UserID, req.ReceiverID, req.Amount, req.Currency)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidAmount):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, service.ErrReceiverNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, service.ErrReceiverInactive):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrInsufficientBalance):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrWalletNotFound):
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "sender or receiver wallet not found"})
		case errors.Is(err, repository.ErrWalletDisabled):
			c.JSON(http.StatusForbidden, dto.ErrorResponse{Error: "sender or receiver wallet is disabled"})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusCreated, dto.NewTransactionResponse(txn))
}

// List handles GET /transactions — admin-only, enforced by
// middleware.RequireAdmin on the route. Powers the admin dashboard's
// Transactions tab.
func (h *TransactionHandler) List(c *gin.Context) {
	filter := repository.TransactionListFilter{
		Limit:  parseIntQuery(c, "limit", 20),
		Offset: parseIntQuery(c, "offset", 0),
	}

	txns, total, err := h.txnService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		return
	}

	c.JSON(http.StatusOK, dto.NewTransactionListResponse(txns, total, filter.Limit, filter.Offset))
}

func parseIntQuery(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
