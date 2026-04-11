// Package recovery implements short-lived signed tokens and the
// deep-link payloads used by the Telegram-based account recovery
// flow (ADR-006).
//
// The package is deliberately small and self-contained so both the
// HTTP handlers and the bot goroutine can import it without pulling
// in the whole handler package.
package recovery

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token purposes. Kept as an exported constant slice so tests and
// the bot handler can switch on them without string typos.
const (
	PurposeLink    = "tg_link"
	PurposeRestore = "tg_restore"
)

// TTL is the lifetime of a recovery token. Sixty seconds is plenty
// for the user to tap the deep link, open Telegram, and hit Start,
// and short enough that a leaked link is effectively dead before
// anything can happen with it.
const TTL = 60 * time.Second

// Claims is the JWT payload. Uses golang-jwt's MapClaims shape in
// practice, but an explicit struct here keeps the purpose field
// typed and lets callers construct tokens without stringly-typed
// maps scattered across handlers.
type Claims struct {
	Subject string    // VPN user id (the one being linked / restored onto)
	Purpose string    // PurposeLink | PurposeRestore
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// ErrInvalidToken wraps every parse/verify failure so callers can
// do errors.Is(err, ErrInvalidToken) without caring about the exact
// JWT library error type.
var ErrInvalidToken = errors.New("invalid recovery token")

// NewToken mints a signed recovery token for the given VPN user id
// and purpose. Returns the compact JWT string ready to embed in a
// Telegram deep link payload.
func NewToken(secret, subject, purpose string) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("recovery: jwt secret is empty")
	}
	if subject == "" {
		return "", fmt.Errorf("recovery: subject is empty")
	}
	if purpose != PurposeLink && purpose != PurposeRestore {
		return "", fmt.Errorf("recovery: unknown purpose %q", purpose)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":     subject,
		"purpose": purpose,
		"iat":     now.Unix(),
		"exp":     now.Add(TTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("recovery: sign: %w", err)
	}
	return signed, nil
}

// ParseToken verifies the signature and expiry on a recovery token
// and returns its claims. Callers must assert the expected purpose
// themselves — ParseToken does not care whether the token is a
// link or a restore, only that it is well-formed and unexpired.
// This keeps the function reusable and puts the purpose check at
// the call site where the reader can see which code path expects
// which purpose.
func ParseToken(secret, raw string) (*Claims, error) {
	if secret == "" {
		return nil, fmt.Errorf("recovery: jwt secret is empty")
	}
	if raw == "" {
		return nil, ErrInvalidToken
	}
	token, err := jwt.Parse(raw, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	sub, _ := mc["sub"].(string)
	purpose, _ := mc["purpose"].(string)
	if sub == "" || purpose == "" {
		return nil, ErrInvalidToken
	}
	if purpose != PurposeLink && purpose != PurposeRestore {
		return nil, ErrInvalidToken
	}
	iat := claimTime(mc, "iat")
	exp := claimTime(mc, "exp")
	// The JWT library already enforces exp, but the redundant check
	// makes the contract explicit and catches any future library
	// regression that silently accepts expired tokens.
	if !exp.IsZero() && time.Now().After(exp) {
		return nil, ErrInvalidToken
	}
	return &Claims{
		Subject:   sub,
		Purpose:   purpose,
		IssuedAt:  iat,
		ExpiresAt: exp,
	}, nil
}

func claimTime(mc jwt.MapClaims, key string) time.Time {
	switch v := mc[key].(type) {
	case float64:
		return time.Unix(int64(v), 0)
	case int64:
		return time.Unix(v, 0)
	case nil:
		return time.Time{}
	}
	return time.Time{}
}
