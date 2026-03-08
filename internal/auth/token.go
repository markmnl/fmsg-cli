// Package auth handles JWT token generation and validation.
package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// devSecret is a fallback secret used only when FMSG_JWT_SECRET is not set.
// WARNING: This is a development placeholder. Set FMSG_JWT_SECRET in production.
const devSecret = "fmsg-dev-secret-do-not-use-in-production"

// jwtSecret returns the signing secret from the environment or falls back to devSecret.
func jwtSecret() []byte {
	if s := os.Getenv("FMSG_JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte(devSecret)
}

// TokenDuration is how long a generated token remains valid.
const TokenDuration = 24 * time.Hour

// Generate creates a signed JWT for the given FMSG address.
// Returns the signed token string and its expiration time.
func Generate(user string) (string, time.Time, error) {
	now := time.Now()
	exp := now.Add(TokenDuration)

	claims := jwt.MapClaims{
		"sub": user,
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing token: %w", err)
	}

	return signed, exp, nil
}

// Validate parses and validates a JWT token string.
// Returns an error if the token is invalid or expired.
func Validate(tokenStr string) error {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return fmt.Errorf("token is not valid")
	}
	return nil
}
