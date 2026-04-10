package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"vpnapp/server/api/internal/cache"
	"vpnapp/server/api/internal/config"
	"vpnapp/server/api/internal/handler"
	"vpnapp/server/api/internal/middleware"
	"vpnapp/server/api/internal/repository"
	"vpnapp/server/api/internal/scheduler"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	stripe "github.com/stripe/stripe-go/v81"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration from environment
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Set Stripe API key once at startup — handlers must not override this.
	stripe.Key = cfg.StripeKey

	// Connect to PostgreSQL
	db, err := repository.NewDB(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	logger.Info("connected to database")

	// Connect to Redis.
	redisClient, err := cache.NewRedisClient(cfg.RedisURL)
	if err != nil {
		logger.Fatal("failed to connect to redis", zap.Error(err))
	}
	logger.Info("connected to redis")

	// Start background session cleanup scheduler.
	scheduler.Start(db, logger, cfg)

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "VPN API Server",
		ServerHeader: "",
		ErrorHandler: handler.ErrorHandler(logger),
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Authorization, X-App-Version",
	}))

	// App version gate — rejects mobile clients below MIN_APP_VERSION.
	// Bypasses are scoped to (method, path) so that for example
	// POST /health is still gated even though GET /health is not.
	app.Use(middleware.AppVersion(
		cfg.MinAppVersion,
		logger,
		middleware.SkipRule{Method: fiber.MethodGet, Path: "/api/v1/health"},
		middleware.SkipRule{Method: fiber.MethodPost, Path: "/api/v1/webhook/stripe"},
		middleware.SkipRule{Method: fiber.MethodPost, Path: "/api/v1/auth/admin-login"},
	))

	// Redis-backed per-user rate limiting. Decodes the JWT (when present) to
	// key on user ID so the limit applies correctly even before auth middleware runs.
	app.Use(middleware.RateLimit(redisClient, logger, cfg.JWTSecret))

	// API routes
	api := app.Group("/api/v1")

	// Public routes (no auth required)
	api.Post("/auth/refresh", handler.RefreshToken(logger, cfg, db))
	api.Post("/auth/guest", handler.GuestLogin(logger, db, cfg))
	api.Post("/auth/admin-login", handler.AdminLogin(logger, cfg, db))
	// /auth/link is intentionally public — the calling device is a brand-new
	// guest that does not yet hold a token for the target account it wants
	// to attach to via the share code. The dedicated rate limiter caps
	// brute-force attempts at 10/minute/IP independently of the global
	// 30/minute/IP bucket so the link endpoint cannot be amortised over
	// other public endpoints.
	api.Post("/auth/link",
		middleware.LinkAttemptLimit(redisClient, logger),
		handler.LinkDevice(logger, cfg, db),
	)
	api.Get("/health", handler.Health())

	// Stripe webhook — public route, authenticated via Stripe-Signature header.
	api.Post("/webhook/stripe", handler.HandleStripeWebhook(logger, cfg, db))

	// Debug endpoint — logs only the "error" and "action" fields from
	// client-side error reports. The body is intentionally NOT logged in
	// full because clients can include sensitive material (device_id,
	// device_secret, tokens) and the request body would otherwise leak
	// into log aggregation. Anything else needed for diagnosis should be
	// added as an explicit field on the client side.
	api.Post("/debug/error", func(c *fiber.Ctx) error {
		var body struct {
			Error  string `json:"error"`
			Action string `json:"action"`
		}
		if err := c.BodyParser(&body); err == nil {
			logger.Warn("CLIENT ERROR REPORT",
				zap.String("error", body.Error),
				zap.String("action", body.Action),
			)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// Protected routes (JWT required)
	authMiddleware := middleware.AuthRequired(cfg.JWTSecret, redisClient)
	protected := api.Group("", authMiddleware)
	protected.Get("/servers", handler.ListServers(logger, db))
	protected.Get("/servers/:id/config", handler.GetServerConfig(logger, db, cfg))
	protected.Get("/subscription", handler.GetSubscription(logger, db))
	protected.Get("/account", handler.GetAccount(logger, db))
	protected.Patch("/account", handler.PatchAccount(logger, db))
	protected.Post("/connections", handler.RegisterConnection(logger, db))
	protected.Delete("/connections/:id", handler.UnregisterConnection(logger, db))
	protected.Get("/connections", handler.ListActiveConnections(logger, db))
	protected.Patch("/connections/:id/heartbeat", handler.HeartbeatConnection(logger, db))
	protected.Post("/subscription/checkout", handler.CreateCheckoutSession(logger, cfg, db))
	protected.Post("/subscription/cancel", handler.CancelSubscription(logger, cfg, db))
	// Plan sharing — owner generates a code, friend's device redeems it via /auth/link.
	protected.Post("/devices/share-code", handler.CreateShareCode(logger, cfg, db))
	protected.Get("/devices", handler.ListMyDevices(logger, db))
	protected.Delete("/devices/:id", handler.DeleteMyDevice(logger, db))

	// Admin routes (JWT + admin role required)
	admin := api.Group("/admin", authMiddleware, middleware.AdminRequired())
	admin.Get("/users", handler.AdminListUsers(logger, db))
	admin.Get("/users/:id", handler.AdminGetUser(logger, db))
	admin.Patch("/users/:id", handler.AdminUpdateUser(logger, db))
	admin.Get("/servers", handler.AdminListServers(logger, db))
	admin.Post("/servers", handler.AdminCreateServer(logger, db))
	admin.Patch("/servers/:id", handler.AdminUpdateServer(logger, db))
	admin.Delete("/servers/:id", handler.AdminDeleteServer(logger, db))
	admin.Get("/stats", handler.AdminGetStats(logger, db))

	// Start server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Port)
		logger.Info("starting API server", zap.String("addr", addr))
		if err := app.Listen(addr); err != nil {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down API server...")

	// Stop accepting new connections first.
	if err := app.Shutdown(); err != nil {
		logger.Error("error during server shutdown", zap.Error(err))
	}

	// Stop the background scheduler.
	scheduler.Stop()
	logger.Info("scheduler stopped")

	// Close database connection.
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Close(); err != nil {
			logger.Error("error closing database", zap.Error(err))
		}
	}
	logger.Info("database connection closed")

	// Close Redis connection.
	if err := redisClient.Close(); err != nil {
		logger.Error("error closing redis", zap.Error(err))
	}
	logger.Info("redis connection closed")
}
