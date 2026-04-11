package handler

// auth_test.go — edge-case tests for AdminLogin, RefreshToken, and GuestLogin handlers.
// These live in package handler (white-box) so they share the same test DB helpers
// already defined in payment_test.go (newTestDB, seedUser, etc.).

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/config"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// authTestApp wraps a single handler in a minimal Fiber app used by auth tests.
func authTestApp(h fiber.Handler) *fiber.App {
	app := fiber.New()
	app.Post("/", h)
	return app
}

// newAuthTestDB opens an in-memory SQLite database with the users, subscriptions,
// and sessions tables needed by the auth handler tests.
func newAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open auth test db: %v", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id                     TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			email_hash              TEXT UNIQUE,
			password_hash           TEXT,
			full_name               TEXT NOT NULL DEFAULT '',
			subscription_tier       TEXT NOT NULL DEFAULT 'free',
			subscription_expires_at DATETIME,
			role                    TEXT NOT NULL DEFAULT 'user',
			telegram_user_id        INTEGER UNIQUE,
			telegram_linked_at      DATETIME,
			telegram_username       TEXT,
			telegram_first_name     TEXT,
			created_at              DATETIME,
			updated_at              DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			user_id    TEXT NOT NULL,
			plan       TEXT NOT NULL DEFAULT 'free',
			stripe_id  TEXT,
			is_active  INTEGER NOT NULL DEFAULT 1,
			started_at DATETIME,
			expires_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			user_id             TEXT NOT NULL,
			refresh_token_hash  TEXT NOT NULL,
			device_info         TEXT,
			created_at          DATETIME,
			expires_at          DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS devices (
			id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
			user_id             TEXT NOT NULL,
			device_id           TEXT NOT NULL UNIQUE,
			device_secret_hash  TEXT,
			platform            TEXT NOT NULL DEFAULT '',
			model               TEXT NOT NULL DEFAULT '',
			first_seen_at       DATETIME,
			last_seen_at        DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS link_codes (
			code        TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			created_at  DATETIME,
			expires_at  DATETIME NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("failed to create auth test table: %v", err)
		}
	}
	return db
}

func doAuthRequest(t *testing.T, app *fiber.App, body interface{}) *http.Response {
	t.Helper()
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(http.MethodPost, "/", reqBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test error: %v", err)
	}
	return resp
}

func testAuthConfig() *config.Config {
	return &config.Config{JWTSecret: "test-secret-32-bytes-at-minimum!!"}
}

// seedAdminUser inserts an admin user directly into the users table and
// returns its email and the plaintext password for use in AdminLogin tests.
func seedAdminUser(t *testing.T, db *gorm.DB) (email, password string) {
	t.Helper()
	email = "admin@vpnapp.local"
	password = "admin-test-password-42"

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))

	if err := db.Exec(
		`INSERT INTO users (email_hash, password_hash, full_name, role, subscription_tier)
		 VALUES (?, ?, 'Admin', 'admin', 'ultimate')`,
		emailHash, string(hash),
	).Error; err != nil {
		t.Fatalf("seeding admin: %v", err)
	}
	return email, password
}

// seedRegularUser inserts a non-admin user for negative AdminLogin tests.
func seedRegularUser(t *testing.T, db *gorm.DB) (email, password string) {
	t.Helper()
	email = "regular@example.com"
	password = "regular-password-42"

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))

	if err := db.Exec(
		`INSERT INTO users (email_hash, password_hash, full_name, role, subscription_tier)
		 VALUES (?, ?, 'User', 'user', 'free')`,
		emailHash, string(hash),
	).Error; err != nil {
		t.Fatalf("seeding user: %v", err)
	}
	return email, password
}

// ---- AdminLogin ----

func TestAdminLogin_EmptyEmail_Returns400(t *testing.T) {
	db := newAuthTestDB(t)
	app := authTestApp(AdminLogin(zap.NewNop(), testAuthConfig(), db))
	resp := doAuthRequest(t, app, map[string]string{"email": "", "password": "any"})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("empty email: expected 400, got %d", resp.StatusCode)
	}
}

func TestAdminLogin_NonAdminUser_Returns401(t *testing.T) {
	db := newAuthTestDB(t)
	email, password := seedRegularUser(t, db)

	app := authTestApp(AdminLogin(zap.NewNop(), testAuthConfig(), db))
	resp := doAuthRequest(t, app, map[string]string{"email": email, "password": password})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("non-admin login: expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminLogin_WrongPassword_Returns401(t *testing.T) {
	db := newAuthTestDB(t)
	email, _ := seedAdminUser(t, db)

	app := authTestApp(AdminLogin(zap.NewNop(), testAuthConfig(), db))
	resp := doAuthRequest(t, app, map[string]string{"email": email, "password": "wrong-password"})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("wrong password: expected 401, got %d", resp.StatusCode)
	}
}

func TestAdminLogin_HappyPath_Returns200WithTokens(t *testing.T) {
	db := newAuthTestDB(t)
	email, password := seedAdminUser(t, db)

	app := authTestApp(AdminLogin(zap.NewNop(), testAuthConfig(), db))
	resp := doAuthRequest(t, app, map[string]string{"email": email, "password": password})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("happy path: expected 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object in login response")
	}
	if data["access_token"] == "" || data["access_token"] == nil {
		t.Error("expected non-empty access_token in login response")
	}
}

// ---- RefreshToken ----

func TestRefreshToken_MissingToken_Returns400(t *testing.T) {
	db := newAuthTestDB(t)
	app := authTestApp(RefreshToken(zap.NewNop(), testAuthConfig(), db))

	resp := doAuthRequest(t, app, map[string]string{})
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("missing refresh_token: expected 400, got %d", resp.StatusCode)
	}
}

func TestRefreshToken_MalformedToken_Returns401(t *testing.T) {
	db := newAuthTestDB(t)
	app := authTestApp(RefreshToken(zap.NewNop(), testAuthConfig(), db))

	resp := doAuthRequest(t, app, map[string]string{
		"refresh_token": "not-a-real-token-at-all",
	})
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("malformed token: expected 401, got %d", resp.StatusCode)
	}
}

// ---- GuestLogin ----

func TestGuestLogin_HappyPath_CreatesUserAndReturnsTokens(t *testing.T) {
	db := newAuthTestDB(t)
	app := authTestApp(GuestLogin(zap.NewNop(), db, testAuthConfig()))

	resp := doAuthRequest(t, app, nil)
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("guest login: expected 201, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&body)
	data, _ := body["data"].(map[string]interface{})
	if data["access_token"] == "" || data["access_token"] == nil {
		t.Error("expected non-empty access_token in guest login response")
	}

	var count int64
	db.Raw("SELECT COUNT(*) FROM users WHERE role = 'user'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 guest user row, got %d", count)
	}
}
