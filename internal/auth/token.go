// Package auth handles JWT token generation and validation.
package auth

import (
	"encoding/base64"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// jwtSecret returns the signing secret from the FMSG_JWT_SECRET environment variable.
// If the value starts with "base64:" the remainder is decoded as base64; otherwise
// the string value is used directly. Returns an error if the variable is not set or empty.
func jwtSecret() ([]byte, error) {
	s := os.Getenv("FMSG_JWT_SECRET")
	if s == "" {
		return nil, fmt.Errorf("FMSG_JWT_SECRET environment variable is required but not set")
	}

	const base64Prefix = "base64:"
	if len(s) >= len(base64Prefix) && s[:len(base64Prefix)] == base64Prefix {
		b, err := base64.StdEncoding.DecodeString(s[len(base64Prefix):])
		if err != nil {
			return nil, fmt.Errorf("invalid base64 JWT secret: %w", err)
		}
		return b, nil
	}

	return []byte(s), nil
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

	secret, err := jwtSecret()
	if err != nil {
		return "", time.Time{}, err
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("signing token: %w", err)
	}

	return signed, exp, nil
}

// Validate parses and validates a JWT token string.
// Returns an error if the token is invalid or expired.
func Validate(tokenStr string) error {
	secret, err := jwtSecret()
	if err != nil {
		return err
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return fmt.Errorf("token is not valid")
	}
	return nil
}
