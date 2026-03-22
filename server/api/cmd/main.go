package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vpnapp/server/api/internal/config"
	"vpnapp/server/api/internal/handler"
	"vpnapp/server/api/internal/middleware"
	"vpnapp/server/api/internal/repository"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
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
	app.Use(limiter.New(limiter.Config{
		Max:        100,
		Expiration: 1 * time.Minute,
	}))

	// API routes
	api := app.Group("/api/v1")

	// Public routes (no auth required)
	api.Post("/auth/register", handler.Register(logger, cfg, db))
	api.Post("/auth/login", handler.Login(logger, cfg, db))
	api.Post("/auth/refresh", handler.RefreshToken(logger, cfg, db))
	api.Get("/health", handler.Health())

	// Protected routes (JWT required)
	protected := api.Group("", middleware.AuthRequired(cfg.JWTSecret))
	protected.Get("/servers", handler.ListServers(logger, db))
	protected.Get("/servers/:id/config", handler.GetServerConfig(logger, db))
	protected.Get("/subscription", handler.GetSubscription(logger, db))
	protected.Get("/account", handler.GetAccount(logger, db))

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
	app.Shutdown()
}
