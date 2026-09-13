// Package handler is the HTTP layer — parses requests, calls service,
// writes responses.
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
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusCreated, dto.NewTransactionResponse(txn))
}
