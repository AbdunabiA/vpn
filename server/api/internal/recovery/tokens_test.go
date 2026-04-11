package recovery_test

import (
	"errors"
	"testing"
	"time"

	"vpnapp/server/api/internal/recovery"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-jwt-secret-32-bytes-minimum!"

func TestNewToken_RoundTrip(t *testing.T) {
	raw, err := recovery.NewToken(testSecret, "user-abc", recovery.PurposeLink)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	claims, err := recovery.ParseToken(testSecret, raw)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.Subject != "user-abc" {
		t.Errorf("subject = %q, want user-abc", claims.Subject)
	}
	if claims.Purpose != recovery.PurposeLink {
		t.Errorf("purpose = %q, want %q", claims.Purpose, recovery.PurposeLink)
	}
	if time.Until(claims.ExpiresAt) < 50*time.Second {
		t.Errorf("expiry too close: %s", time.Until(claims.ExpiresAt))
	}
}

func TestNewToken_RejectsUnknownPurpose(t *testing.T) {
	_, err := recovery.NewToken(testSecret, "user-abc", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown purpose, got nil")
	}
}

func TestNewToken_RejectsEmptySubject(t *testing.T) {
	_, err := recovery.NewToken(testSecret, "", recovery.PurposeLink)
	if err == nil {
		t.Fatal("expected error for empty subject, got nil")
	}
}

func TestParseToken_RejectsTamperedSignature(t *testing.T) {
	raw, err := recovery.NewToken(testSecret, "user-abc", recovery.PurposeLink)
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	// Flip the last character of the signature segment.
	b := []byte(raw)
	b[len(b)-1] ^= 1
	if _, err := recovery.ParseToken(testSecret, string(b)); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("tampered token should return ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_RejectsWrongSecret(t *testing.T) {
	raw, _ := recovery.NewToken(testSecret, "user-abc", recovery.PurposeLink)
	if _, err := recovery.ParseToken("different-secret-also-32-bytes!!!", raw); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("wrong secret should return ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_RejectsExpiredToken(t *testing.T) {
	// Construct a JWT directly with an expiry in the past so we do
	// not have to sleep for the real TTL in tests.
	claims := jwt.MapClaims{
		"sub":     "user-abc",
		"purpose": recovery.PurposeLink,
		"iat":     time.Now().Add(-2 * time.Minute).Unix(),
		"exp":     time.Now().Add(-1 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := recovery.ParseToken(testSecret, raw); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("expired token should return ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_RejectsEmpty(t *testing.T) {
	if _, err := recovery.ParseToken(testSecret, ""); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("empty token should return ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_RejectsWrongAlgorithm(t *testing.T) {
	// Build a "none"-algorithm token by hand-crafting headers. The
	// library should reject it because we enforce HMAC only.
	claims := jwt.MapClaims{
		"sub":     "user-abc",
		"purpose": recovery.PurposeLink,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	raw, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := recovery.ParseToken(testSecret, raw); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("none-algorithm token should return ErrInvalidToken, got %v", err)
	}
}

func TestParseToken_RejectsUnknownPurposeInClaims(t *testing.T) {
	// Hand-craft a token with a bogus purpose claim, signed
	// correctly — the ParseToken purpose switch must still reject it.
	claims := jwt.MapClaims{
		"sub":     "user-abc",
		"purpose": "bogus_purpose",
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	raw, _ := tok.SignedString([]byte(testSecret))
	if _, err := recovery.ParseToken(testSecret, raw); !errors.Is(err, recovery.ErrInvalidToken) {
		t.Errorf("unknown-purpose token should return ErrInvalidToken, got %v", err)
	}
}

func TestNewToken_LinkAndRestoreDiffer(t *testing.T) {
	l, _ := recovery.NewToken(testSecret, "user-abc", recovery.PurposeLink)
	r, _ := recovery.NewToken(testSecret, "user-abc", recovery.PurposeRestore)
	if l == r {
		t.Error("link and restore tokens for same subject should differ")
	}
	lc, _ := recovery.ParseToken(testSecret, l)
	rc, _ := recovery.ParseToken(testSecret, r)
	if lc.Purpose != recovery.PurposeLink || rc.Purpose != recovery.PurposeRestore {
		t.Errorf("purpose mismatch: link=%s restore=%s", lc.Purpose, rc.Purpose)
	}
}
