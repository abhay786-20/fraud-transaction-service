// Package middleware holds Gin middleware — cross-cutting checks that
// apply identically across many routes.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/abhay786-20/fraud-transaction-service/internal/dto"
	"github.com/abhay786-20/fraud-transaction-service/pkg/token"
)

const contextKeyClaims = "claims"

// Auth validates the "Authorization: Bearer <token>" header against the
// SAME secret fraud-auth-service signs tokens with — this is the shared-
// secret approach we deliberately chose for now (see coding-plan.md §5).
// Operational note: if this service's TRANSACTION_JWT_SECRET ever drifts
// out of sync with fraud-auth-service's AUTH_JWT_SECRET, every request
// here starts failing with "invalid or expired token" — same value,
// different env var names, kept in sync manually.
func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		const prefix = "Bearer "
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, prefix) {
			c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "missing or malformed authorization header"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(header, prefix)
		claims, err := token.Parse(tokenString, jwtSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, dto.ErrorResponse{Error: "invalid or expired token"})
			c.Abort()
			return
		}

		c.Set(contextKeyClaims, claims)
		c.Next()
	}
}

// GetClaims retrieves the claims Auth stored on this request. Returns nil
// if Auth never ran (or failed).
func GetClaims(c *gin.Context) *token.Claims {
	value, exists := c.Get(contextKeyClaims)
	if !exists {
		return nil
	}
	claims, ok := value.(*token.Claims)
	if !ok {
		return nil
	}
	return claims
}
