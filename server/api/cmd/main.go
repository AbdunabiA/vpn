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
	scheduler.Start(db, logger)

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
		AllowHeaders: "Origin, Content-Type, Authorization",
	}))
	// Redis-backed per-user rate limiting replaces the previous global limiter.
	app.Use(middleware.RateLimit(redisClient, logger))

	// API routes
	api := app.Group("/api/v1")

	// Public routes (no auth required)
	api.Post("/auth/register", handler.Register(logger, cfg, db))
	api.Post("/auth/login", handler.Login(logger, cfg, db))
	api.Post("/auth/refresh", handler.RefreshToken(logger, cfg, db))
	api.Get("/health", handler.Health())

	// Stripe webhook — public route, authenticated via Stripe-Signature header.
	api.Post("/webhook/stripe", handler.HandleStripeWebhook(logger, cfg, db))

	// Protected routes (JWT required)
	authMiddleware := middleware.AuthRequired(cfg.JWTSecret, redisClient)
	protected := api.Group("", authMiddleware)
	protected.Get("/servers", handler.ListServers(logger, db))
	protected.Get("/servers/:id/config", handler.GetServerConfig(logger, db))
	protected.Get("/subscription", handler.GetSubscription(logger, db))
	protected.Get("/account", handler.GetAccount(logger, db))
	protected.Post("/connections", handler.RegisterConnection(logger, db))
	protected.Delete("/connections/:id", handler.UnregisterConnection(logger, db))
	protected.Get("/connections", handler.ListActiveConnections(logger, db))
	protected.Post("/subscription/checkout", handler.CreateCheckoutSession(logger, cfg, db))
	protected.Post("/subscription/cancel", handler.CancelSubscription(logger, cfg, db))

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

	// Close Redis connection.
	if err := redisClient.Close(); err != nil {
		logger.Error("error closing redis", zap.Error(err))
	}
	logger.Info("redis connection closed")
}
