package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpnapp/server/api/internal/handler"
	"vpnapp/server/api/internal/model"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newServersTestDB opens an in-memory SQLite database for server handler tests.
// Contains the vpn_servers and connections tables (same schema as connection_test.go).
func newServersTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	ddl := `
		CREATE TABLE IF NOT EXISTS vpn_servers (
			id TEXT PRIMARY KEY,
			hostname TEXT NOT NULL UNIQUE,
			ip_address TEXT NOT NULL,
			region TEXT NOT NULL,
			city TEXT NOT NULL,
			country TEXT NOT NULL,
			country_code TEXT NOT NULL,
			protocol TEXT NOT NULL DEFAULT 'vless-reality',
			capacity INTEGER NOT NULL DEFAULT 500,
			current_load INTEGER NOT NULL DEFAULT 0,
			is_active INTEGER NOT NULL DEFAULT 1,
			reality_public_key TEXT,
			reality_short_id TEXT,
			ws_enabled INTEGER NOT NULL DEFAULT 0,
			ws_host TEXT,
			ws_path TEXT DEFAULT '/ws',
			awg_public_key TEXT,
			awg_endpoint TEXT,
			awg_params TEXT,
			created_at DATETIME
		);
		CREATE TABLE IF NOT EXISTS connections (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			server_id TEXT NOT NULL,
			connected_at DATETIME,
			disconnected_at DATETIME,
			bytes_up INTEGER NOT NULL DEFAULT 0,
			bytes_down INTEGER NOT NULL DEFAULT 0
		);
	`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	return db
}

// buildServerConfigApp constructs a Fiber app with the GetServerConfig route.
func buildServerConfigApp(db *gorm.DB, userID, tier string) *fiber.App {
	log := zap.NewNop()
	app := fiber.New()

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user_id", userID)
		c.Locals("tier", tier)
		return c.Next()
	})

	app.Get("/servers/:id/config", handler.GetServerConfig(log, db))
	return app
}

// seedServer inserts a VPNServer with configurable fields.
func seedServer(t *testing.T, db *gorm.DB, srv *model.VPNServer) *model.VPNServer {
	t.Helper()
	if srv.ID == "" {
		srv.ID = uuid.NewString()
	}
	if err := db.Create(srv).Error; err != nil {
		t.Fatalf("seedServer: %v", err)
	}
	return srv
}

// getServerConfig fires GET /servers/:id/config and returns the decoded response.
func getServerConfig(t *testing.T, app *fiber.App, serverID string) (*http.Response, map[string]interface{}) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/servers/"+serverID+"/config", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil && err != io.EOF {
		t.Fatalf("failed to decode response body: %v", err)
	}
	return resp, body
}

// --- GetServerConfig AWG tests ---

func TestGetServerConfig_NoAWG_OmitsAWGField(t *testing.T) {
	db := newServersTestDB(t)
	srv := seedServer(t, db, &model.VPNServer{
		Hostname:         "test-no-awg",
		IPAddress:        "10.0.0.1",
		Region:           "EU",
		City:             "Berlin",
		Country:          "Germany",
		CountryCode:      "DE",
		Protocol:         "vless-reality",
		IsActive:         true,
		RealityPublicKey: "reality-public-key",
		RealityShortID:   "abcd1234",
	})

	app := buildServerConfigApp(db, "user-123", "premium")
	resp, body := getServerConfig(t, app, srv.ID)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %v", resp.StatusCode, body)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing 'data' object")
	}

	// When the server has no AWG keys, the "awg" field must be absent (omitempty).
	if _, hasAWG := data["awg"]; hasAWG {
		t.Error("expected 'awg' to be absent when server has no AWG configuration")
	}
}

func TestGetServerConfig_WithAWG_IncludesAWGField(t *testing.T) {
	db := newServersTestDB(t)

	pubKey := "awg-public-key-base64=="
	endpoint := "10.0.0.1:51820"
	awgParamsJSON := `{"jc":5,"jmin":50,"jmax":1000,"s1":10,"s2":20,"h1":1,"h2":2,"h3":3,"h4":4}`

	// Insert via raw SQL to avoid GORM JSONB serializer issues in SQLite test env.
	if err := db.Exec(`
		INSERT INTO vpn_servers
			(id, hostname, ip_address, region, city, country, country_code,
			 protocol, is_active, reality_public_key, reality_short_id,
			 awg_public_key, awg_endpoint, awg_params)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), "test-with-awg", "10.0.0.1",
		"EU", "Amsterdam", "Netherlands", "NL",
		"vless-reality", 1,
		"TestRealityPublicKey123456789012345678901234", "abcd1234",
		pubKey, endpoint, awgParamsJSON,
	).Error; err != nil {
		t.Fatalf("failed to seed AWG server: %v", err)
	}

	// Fetch the inserted server ID.
	var srvID string
	db.Raw("SELECT id FROM vpn_servers WHERE hostname = ?", "test-with-awg").Scan(&srvID)
	if srvID == "" {
		t.Fatal("could not retrieve seeded server ID")
	}

	app := buildServerConfigApp(db, "user-456", "premium")
	resp, body := getServerConfig(t, app, srvID)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %v", resp.StatusCode, body)
	}

	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response missing 'data' object; body=%v", body)
	}

	awg, hasAWG := data["awg"]
	if !hasAWG {
		t.Fatal("expected 'awg' field in response when server has AWG configuration")
	}

	awgMap, ok := awg.(map[string]interface{})
	if !ok {
		t.Fatalf("'awg' field is not an object; got %T", awg)
	}

	if awgMap["public_key"] != pubKey {
		t.Errorf("expected public_key=%q, got %q", pubKey, awgMap["public_key"])
	}
	if awgMap["endpoint"] != endpoint {
		t.Errorf("expected endpoint=%q, got %q", endpoint, awgMap["endpoint"])
	}
	if awgMap["allowed_ips"] != "0.0.0.0/0, ::/0" {
		t.Errorf("expected allowed_ips='0.0.0.0/0, ::/0', got %q", awgMap["allowed_ips"])
	}
}

func TestGetServerConfig_WithAWG_AllowedIPsIsFullTunnel(t *testing.T) {
	db := newServersTestDB(t)

	pubKey := "awg-pk=="
	if err := db.Exec(`
		INSERT INTO vpn_servers
			(id, hostname, ip_address, region, city, country, country_code,
			 protocol, is_active, reality_public_key, reality_short_id,
			 awg_public_key, awg_endpoint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.NewString(), "test-awg-ips", "1.2.3.4",
		"AS", "Tokyo", "Japan", "JP",
		"vless-reality", 1,
		"TestRealityPublicKey123456789012345678901234", "abcd1234",
		pubKey, "1.2.3.4:51820",
	).Error; err != nil {
		t.Fatalf("failed to seed: %v", err)
	}

	var srvID string
	db.Raw("SELECT id FROM vpn_servers WHERE hostname = ?", "test-awg-ips").Scan(&srvID)

	app := buildServerConfigApp(db, "user-789", "premium")
	resp, body := getServerConfig(t, app, srvID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	data := body["data"].(map[string]interface{})
	awgMap := data["awg"].(map[string]interface{})

	// The client must route all traffic through the tunnel.
	allowedIPs, _ := awgMap["allowed_ips"].(string)
	if allowedIPs == "" {
		t.Error("allowed_ips must not be empty")
	}
}

func TestGetServerConfig_NotFound(t *testing.T) {
	db := newServersTestDB(t)
	app := buildServerConfigApp(db, "user-x", "premium")

	resp, _ := getServerConfig(t, app, uuid.NewString())
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetServerConfig_InactiveServer(t *testing.T) {
	db := newServersTestDB(t)

	// Insert via raw SQL with is_active=0 — GORM skips zero-value booleans on Create
	// so we bypass it here to reliably seed an inactive server in SQLite.
	srvID := uuid.NewString()
	if err := db.Exec(`
		INSERT INTO vpn_servers
			(id, hostname, ip_address, region, city, country, country_code, protocol, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		srvID, "inactive-srv", "10.0.0.1", "EU", "Paris", "France", "FR", "vless-reality",
	).Error; err != nil {
		t.Fatalf("failed to seed inactive server: %v", err)
	}

	app := buildServerConfigApp(db, "user-y", "premium")
	resp, _ := getServerConfig(t, app, srvID)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for inactive server, got %d", resp.StatusCode)
	}
}

func TestGetServerConfig_DeviceLimitReached(t *testing.T) {
	db := newServersTestDB(t)
	srv := seedServer(t, db, &model.VPNServer{
		Hostname:    "limit-test-srv",
		IPAddress:   "10.0.0.1",
		Region:      "EU",
		City:        "Madrid",
		Country:     "Spain",
		CountryCode: "ES",
		Protocol:    "vless-reality",
		IsActive:    true,
	})

	userID := "user-at-limit"

	// Insert active connections to reach free tier limit (1 device).
	for i := 0; i < 1; i++ {
		db.Exec(`INSERT INTO connections (id, user_id, server_id, connected_at)
			VALUES (?, ?, ?, datetime('now'))`,
			uuid.NewString(), userID, srv.ID)
	}

	// Free tier allows 1 device; with 1 active connection the limit is reached.
	app := buildServerConfigApp(db, userID, "free")
	resp, body := getServerConfig(t, app, srv.ID)

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d; body: %v", resp.StatusCode, body)
	}
}

// --- AWGClientConfig struct field tests ---

func TestAWGClientConfigHasAllRequiredFields(t *testing.T) {
	cfg := handler.AWGClientConfig{
		PublicKey:  "pk==",
		Endpoint:   "1.2.3.4:51820",
		AllowedIPs: "0.0.0.0/0, ::/0",
		Jc:         5,
		Jmin:       50,
		Jmax:       1000,
		S1:         10,
		S2:         20,
		H1:         1,
		H2:         2,
		H3:         3,
		H4:         4,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal AWGClientConfig: %v", err)
	}

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	required := []string{
		"public_key", "endpoint", "allowed_ips",
		"jc", "jmin", "jmax",
		"s1", "s2", "h1", "h2", "h3", "h4",
	}
	for _, field := range required {
		if _, ok := m[field]; !ok {
			t.Errorf("AWGClientConfig missing JSON field %q", field)
		}
	}
}
