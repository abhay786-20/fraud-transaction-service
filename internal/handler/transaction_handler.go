// Package handler is the HTTP layer — parses requests, calls service,
// writes responses.
package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
		Status: c.Query("status"),
		Search: c.Query("search"),
		Limit:  parseIntQuery(c, "limit", 20),
		Offset: parseIntQuery(c, "offset", 0),
	}
	if idsStr := c.Query("user_ids"); idsStr != "" {
		filter.SearchUserIDs = strings.Split(idsStr, ",")
	}

	txns, total, err := h.txnService.List(c.Request.Context(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		return
	}

	c.JSON(http.StatusOK, dto.NewTransactionListResponse(txns, total, filter.Limit, filter.Offset))
}

// Flag handles POST /transactions/:id/flag — admin-only, enforced by
// middleware.RequireAdmin on the route. Lets an admin manually flag an
// already-completed transaction as fraudulent — the case fraud-engine-
// service's automatic scoring missed — which notifies sender, receiver,
// and every admin exactly like an automatic fraud alert would.
func (h *TransactionHandler) Flag(c *gin.Context) {
	// Body is optional — Reason has no binding tag, so an empty or missing
	// body just means "use the default reason," not a request error.
	var req dto.FlagTransactionRequest
	_ = c.ShouldBindJSON(&req)

	txn, err := h.txnService.Flag(c.Request.Context(), c.Param("id"), req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTransactionNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrTransactionNotFlaggable):
			c.JSON(http.StatusConflict, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewTransactionResponse(txn))
}

// Unflag handles POST /transactions/:id/unflag — admin-only, enforced by
// middleware.RequireAdmin on the route. Lets an admin reverse a manual or
// automatic fraud flag that turned out to be a false alarm, which
// notifies sender, receiver, and every admin that the transaction was
// reviewed and cleared.
func (h *TransactionHandler) Unflag(c *gin.Context) {
	var req dto.FlagTransactionRequest
	_ = c.ShouldBindJSON(&req)

	txn, err := h.txnService.Unflag(c.Request.Context(), c.Param("id"), req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrTransactionNotFound):
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		case errors.Is(err, repository.ErrTransactionNotFlagged):
			c.JSON(http.StatusConflict, dto.ErrorResponse{Error: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "something went wrong"})
		}
		return
	}

	c.JSON(http.StatusOK, dto.NewTransactionResponse(txn))
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
