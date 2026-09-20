// Package router registers every HTTP route this service exposes.
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/abhay786-20/fraud-transaction-service/internal/handler"
	"github.com/abhay786-20/fraud-transaction-service/internal/middleware"
)

// New builds the full set of routes for this service.
func New(pg *sqlx.DB, jwtSecret string, txnHandler *handler.TransactionHandler, walletHandler *handler.WalletHandler) *gin.Engine {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/ready", func(c *gin.Context) {
		if err := pg.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready", "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	auth := middleware.Auth(jwtSecret)
	requireAdmin := middleware.RequireAdmin()
	r.POST("/transactions", auth, txnHandler.Create)
	r.GET("/transactions", auth, requireAdmin, txnHandler.List)
	r.POST("/transactions/:id/flag", auth, requireAdmin, txnHandler.Flag)
	r.POST("/transactions/:id/unflag", auth, requireAdmin, txnHandler.Unflag)

	r.POST("/wallets", auth, walletHandler.Create)
	r.GET("/wallets/me", auth, walletHandler.GetMine)
	r.POST("/wallets/topup", auth, walletHandler.TopUp)
	r.GET("/wallets", auth, requireAdmin, walletHandler.List)
	r.PATCH("/wallets/:userId", auth, requireAdmin, walletHandler.SetEnabled)

	return r
}
