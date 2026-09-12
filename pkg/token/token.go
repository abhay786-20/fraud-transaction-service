// Package token verifies JWTs issued by fraud-auth-service. This service
// never issues tokens itself — only fraud-auth-service is an identity
// provider — so unlike its pkg/token, there is no Generate function here,
// only Parse.
package token

import "github.com/golang-jwt/jwt/v5"

// Claims mirrors fraud-auth-service's pkg/token.Claims exactly — both
// services must agree on this shape, since they share the same signing
// secret (see config's JWTSecret and the operational note in
// internal/middleware/jwt_middleware.go).
type Claims struct {
	UserID string `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// Parse validates a JWT string (signature + expiry) and returns its claims.
func Parse(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}

	parsed, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if !parsed.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}
