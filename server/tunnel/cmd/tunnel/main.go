package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"vpnapp/server/tunnel/internal"

	"go.uber.org/zap"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.json", "Path to server configuration file")
	flag.Parse()

	// Initialize structured logger
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load server configuration
	config, err := internal.LoadConfig(*configPath)
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}

	// Create and start the tunnel server
	server, err := internal.NewTunnelServer(config, logger)
	if err != nil {
		logger.Fatal("failed to create tunnel server", zap.Error(err))
	}

	if err := server.Start(); err != nil {
		logger.Fatal("failed to start tunnel server", zap.Error(err))
	}

	logger.Info("tunnel server started",
		zap.String("listen", fmt.Sprintf(":%d", config.Port)),
		zap.String("protocol", config.Protocol),
	)

	// Wait for shutdown signal (SIGINT or SIGTERM)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown
	if err := server.Stop(); err != nil {
		logger.Error("error during shutdown", zap.Error(err))
	}

	logger.Info("tunnel server stopped")
}
