package recovery

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Telegram bot deep-link /start payloads are limited to 64 characters
// and only accept [A-Za-z0-9_-]. That's too small and too restrictive
// for a JWT (which averages ~200 chars and contains "."), so we use
// an opaque short-lived token stored in Redis instead of a signed
// JWT. The token is single-use: ConsumeStartToken atomically reads
// and deletes the entry via GETDEL so a replay is impossible even
// under concurrent bot updates.

// Purpose values distinguish link from restore tokens. Persisted
// alongside the subject so the bot's /start handler can verify
// the payload matches the code path it was intended for.
const (
	PurposeLink    = "tg_link"
	PurposeRestore = "tg_restore"
)

// StartTokenPayload is what ConsumeStartToken returns. Mirrors the
// JWT Claims struct but sourced from Redis rather than an HMAC
// signature.
type StartTokenPayload struct {
	Subject string // VPN user id
	Purpose string // PurposeLink | PurposeRestore
}

// StartTokenTTL is how long a minted deep-link token stays valid
// before Redis expires it. Same 60 s as the old JWT TTL.
const StartTokenTTL = 60 * time.Second

// startTokenBytes is the raw entropy size. 24 bytes → 32 characters
// of base64url (no padding) which fits easily inside Telegram's
// 64-char cap even after the "link_" / "restore_" prefix.
const startTokenBytes = 24

// ErrExpiredOrUnknownToken wraps the three "token no longer valid"
// cases — expired, never existed, already consumed.
var ErrExpiredOrUnknownToken = errors.New("recovery: start token expired or unknown")

// startTokenKey returns the Redis key used to store a token's
// payload. Namespaced to "tg_start:" so the rest of the Redis
// database remains unaffected.
func startTokenKey(token string) string {
	return "tg_start:" + token
}

// MintStartToken generates a new opaque token, stores the given
// subject + purpose in Redis for StartTokenTTL, and returns the
// token string ready to embed in a deep-link /start payload.
//
// Must be called with a bounded-lifetime context (the HTTP request
// context is fine). If Redis is unreachable the call fails and
// the handler surfaces a 500 to the caller — the mobile app
// retries, no data is corrupted.
func MintStartToken(ctx context.Context, rdb *redis.Client, subject, purpose string) (string, error) {
	if rdb == nil {
		return "", fmt.Errorf("recovery: redis client is nil")
	}
	if subject == "" {
		return "", fmt.Errorf("recovery: subject is empty")
	}
	if purpose != PurposeLink && purpose != PurposeRestore {
		return "", fmt.Errorf("recovery: unknown purpose %q", purpose)
	}

	raw := make([]byte, startTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("recovery: generate token bytes: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw) // 32 chars, no "."

	// Value format: "<purpose>:<subject>". Both fields are plain
	// ASCII so a simple colon split is enough — no need for JSON.
	value := purpose + ":" + subject
	if err := rdb.Set(ctx, startTokenKey(token), value, StartTokenTTL).Err(); err != nil {
		return "", fmt.Errorf("recovery: store token: %w", err)
	}
	return token, nil
}

// ConsumeStartToken atomically looks up and deletes a start token,
// returning its stored payload. Uses GETDEL (Redis 6.2+) so a
// replay of the same token returns ErrExpiredOrUnknownToken even
// if the bot processes two /start updates with the same payload
// in quick succession.
func ConsumeStartToken(ctx context.Context, rdb *redis.Client, token string) (*StartTokenPayload, error) {
	if rdb == nil {
		return nil, fmt.Errorf("recovery: redis client is nil")
	}
	if token == "" {
		return nil, ErrExpiredOrUnknownToken
	}
	val, err := rdb.GetDel(ctx, startTokenKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrExpiredOrUnknownToken
		}
		return nil, fmt.Errorf("recovery: redis getdel: %w", err)
	}
	// Split at the first colon — subject is a UUID so it cannot
	// contain a colon, and purpose is one of the two fixed values.
	for i := 0; i < len(val); i++ {
		if val[i] == ':' {
			purpose := val[:i]
			subject := val[i+1:]
			if purpose != PurposeLink && purpose != PurposeRestore {
				return nil, ErrExpiredOrUnknownToken
			}
			if subject == "" {
				return nil, ErrExpiredOrUnknownToken
			}
			return &StartTokenPayload{Subject: subject, Purpose: purpose}, nil
		}
	}
	return nil, ErrExpiredOrUnknownToken
}
