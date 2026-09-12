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
func New(pg *sqlx.DB, jwtSecret string, txnHandler *handler.TransactionHandler) *gin.Engine {
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
	r.POST("/transactions", auth, txnHandler.Create)

	return r
}
