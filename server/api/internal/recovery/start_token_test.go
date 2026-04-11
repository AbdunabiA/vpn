package recovery_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"vpnapp/server/api/internal/recovery"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func TestMintStartToken_RoundTrip(t *testing.T) {
	ctx := context.Background()
	rdb := newTestRedis(t)

	token, err := recovery.MintStartToken(ctx, rdb, "user-abc", recovery.PurposeLink)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if len(token) < 20 || len(token) > 60 {
		t.Errorf("token length %d outside expected range", len(token))
	}
	for _, c := range token {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			t.Errorf("token contains invalid character %q for Telegram deep link", c)
		}
	}

	payload, err := recovery.ConsumeStartToken(ctx, rdb, token)
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if payload.Subject != "user-abc" {
		t.Errorf("subject = %q, want user-abc", payload.Subject)
	}
	if payload.Purpose != recovery.PurposeLink {
		t.Errorf("purpose = %q, want %q", payload.Purpose, recovery.PurposeLink)
	}
}

func TestConsumeStartToken_SingleUse(t *testing.T) {
	ctx := context.Background()
	rdb := newTestRedis(t)

	token, _ := recovery.MintStartToken(ctx, rdb, "user-abc", recovery.PurposeRestore)
	if _, err := recovery.ConsumeStartToken(ctx, rdb, token); err != nil {
		t.Fatalf("first consume failed: %v", err)
	}
	// Second consume must return ErrExpiredOrUnknownToken — GETDEL
	// deleted the key on the first call, so the replay cannot succeed.
	_, err := recovery.ConsumeStartToken(ctx, rdb, token)
	if !errors.Is(err, recovery.ErrExpiredOrUnknownToken) {
		t.Errorf("second consume: expected ErrExpiredOrUnknownToken, got %v", err)
	}
}

func TestConsumeStartToken_UnknownToken(t *testing.T) {
	ctx := context.Background()
	rdb := newTestRedis(t)
	_, err := recovery.ConsumeStartToken(ctx, rdb, "never-minted-token")
	if !errors.Is(err, recovery.ErrExpiredOrUnknownToken) {
		t.Errorf("unknown token: expected ErrExpiredOrUnknownToken, got %v", err)
	}
}

func TestConsumeStartToken_EmptyToken(t *testing.T) {
	ctx := context.Background()
	rdb := newTestRedis(t)
	_, err := recovery.ConsumeStartToken(ctx, rdb, "")
	if !errors.Is(err, recovery.ErrExpiredOrUnknownToken) {
		t.Errorf("empty token: expected ErrExpiredOrUnknownToken, got %v", err)
	}
}

func TestMintStartToken_RejectsBadPurpose(t *testing.T) {
	ctx := context.Background()
	rdb := newTestRedis(t)
	_, err := recovery.MintStartToken(ctx, rdb, "user-abc", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown purpose, got nil")
	}
}

func TestStartTokenTTL(t *testing.T) {
	// Sanity check the exported TTL — the Telegram deep-link flow
	// depends on this being "long enough for a user to tap through
	// but short enough that a leak is effectively dead". If anyone
	// ever bumps it above, say, 5 minutes without thinking, this
	// test should make them pause.
	if recovery.StartTokenTTL < 30*time.Second || recovery.StartTokenTTL > 2*time.Minute {
		t.Errorf("StartTokenTTL = %s is outside the acceptable 30s..2m window", recovery.StartTokenTTL)
	}
}
