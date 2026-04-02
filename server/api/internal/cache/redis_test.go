package cache_test

import (
	"context"
	"testing"
	"time"

	"vpnapp/server/api/internal/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestClient spins up a miniredis server and returns a connected client.
// The server is automatically closed when the test ends.
func newTestClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

// --- NewRedisClient ---

func TestNewRedisClient_ValidURL(t *testing.T) {
	mr := miniredis.RunT(t)
	client, err := cache.NewRedisClient("redis://" + mr.Addr())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	_ = client.Close()
}

func TestNewRedisClient_InvalidURL(t *testing.T) {
	_, err := cache.NewRedisClient("not://a-valid-redis-url")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestNewRedisClient_UnreachableServer(t *testing.T) {
	// Port 1 is never open — connection will be refused immediately.
	_, err := cache.NewRedisClient("redis://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error for unreachable server, got nil")
	}
}

// --- IsTokenBlacklisted / BlacklistToken ---

func TestBlacklistToken_ThenIsBlacklisted(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	const hash = "deadbeef1234"

	// Token must not be blacklisted before we add it.
	if cache.IsTokenBlacklisted(ctx, client, hash) {
		t.Fatal("expected token to not be blacklisted yet")
	}

	if err := cache.BlacklistToken(ctx, client, hash, 5*time.Minute); err != nil {
		t.Fatalf("BlacklistToken returned error: %v", err)
	}

	if !cache.IsTokenBlacklisted(ctx, client, hash) {
		t.Fatal("expected token to be blacklisted after BlacklistToken call")
	}
}

func TestIsTokenBlacklisted_UnknownHash(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	if cache.IsTokenBlacklisted(ctx, client, "nonexistent") {
		t.Fatal("expected false for unknown token hash")
	}
}

func TestBlacklistToken_ExpiresAfterTTL(t *testing.T) {
	ctx := context.Background()
	client, mr := newTestClient(t)

	const hash = "expiring-hash"
	if err := cache.BlacklistToken(ctx, client, hash, 1*time.Second); err != nil {
		t.Fatalf("BlacklistToken returned error: %v", err)
	}

	if !cache.IsTokenBlacklisted(ctx, client, hash) {
		t.Fatal("expected token to be blacklisted immediately after setting it")
	}

	// Fast-forward the miniredis clock past the TTL.
	mr.FastForward(2 * time.Second)

	if cache.IsTokenBlacklisted(ctx, client, hash) {
		t.Fatal("expected token to no longer be blacklisted after TTL expiry")
	}
}

// --- IncrRateLimit ---

func TestIncrRateLimit_CounterIncrementsCorrectly(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	for i := int64(1); i <= 5; i++ {
		count, err := cache.IncrRateLimit(ctx, client, "user:test-user", time.Minute)
		if err != nil {
			t.Fatalf("IncrRateLimit call %d returned error: %v", i, err)
		}
		if count != i {
			t.Fatalf("expected count %d, got %d", i, count)
		}
	}
}

func TestIncrRateLimit_DifferentKeysDontInterfere(t *testing.T) {
	ctx := context.Background()
	client, _ := newTestClient(t)

	countA, err := cache.IncrRateLimit(ctx, client, "user:alice", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	countB, err := cache.IncrRateLimit(ctx, client, "user:bob", time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if countA != 1 || countB != 1 {
		t.Fatalf("expected both counters to start at 1, got alice=%d bob=%d", countA, countB)
	}
}

func TestIncrRateLimit_KeyExpiresAfterWindow(t *testing.T) {
	ctx := context.Background()
	client, mr := newTestClient(t)

	_, err := cache.IncrRateLimit(ctx, client, "ip:1.2.3.4", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fast-forward past the window.
	mr.FastForward(2 * time.Second)

	// Counter should reset to 1 after expiry.
	count, err := cache.IncrRateLimit(ctx, client, "ip:1.2.3.4", 1*time.Second)
	if err != nil {
		t.Fatalf("unexpected error after key expiry: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected counter to reset to 1 after TTL expiry, got %d", count)
	}
}
