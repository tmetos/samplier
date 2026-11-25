// Command samplier runs the Samplier HTTP API.
//
// Usage:
//
//	samplier serve          # HTTP API (default)
//	samplier mcp            # MCP server over stdio
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/samplier/samplier/internal/app"
	"github.com/samplier/samplier/internal/basemap"
	"github.com/samplier/samplier/internal/config"
	"github.com/samplier/samplier/internal/db"
	"github.com/samplier/samplier/internal/mcp"
	"github.com/samplier/samplier/internal/store"
	"github.com/samplier/samplier/internal/telemetry"
)

// Version is the build version, overridable with -ldflags.
var Version = "0.1.0"

func main() {
	// Emit and interpret every timestamp as UTC. Environmental data is
	// date-centric; a server-local timezone would shift calendar dates.
	time.Local = time.UTC

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	cfg := config.FromEnv()

	otelShutdown, err := telemetry.Init(context.Background())
	if err != nil {
		slog.Error("telemetry init failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = otelShutdown(context.Background()) }()

	logger := telemetry.NewLogger()
	slog.SetDefault(logger)

	switch command {
	case "serve":
		serve(cfg, logger)
	case "mcp":
		serveMCP(cfg, logger)
	case "version":
		logger.Info("samplier", "version", Version)
	default:
		logger.Error("unknown command", "command", command,
			"usage", "samplier [serve|mcp|version]")
		os.Exit(2)
	}
}

// connect opens the database pool or exits.
func connect(ctx context.Context, cfg config.Config, logger *slog.Logger) *pgxpool.Pool {
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	return pool
}

func serve(cfg config.Config, logger *slog.Logger) {
	ctx := context.Background()
	pool := connect(ctx, cfg, logger)
	defer pool.Close()

	srv := app.New(pool, app.Config{
		AuthEnabled:    cfg.AuthEnabled,
		OIDCIssuer:     cfg.OIDCIssuer,
		OIDCClientID:   cfg.OIDCClientID,
		MaxUploadBytes: cfg.MaxUploadBytes,
		Version:        Version,
		Basemap: basemap.Config{
			Provider:  cfg.BasemapProvider,
			TileURL:   cfg.BasemapTileURL,
			GlyphsURL: cfg.BasemapGlyphsURL,
			SpriteURL: cfg.BasemapSpriteURL,
		},
	}, logger)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("samplier API listening",
			"port", cfg.Port, "version", Version, "auth_enabled", cfg.AuthEnabled)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("shutdown incomplete", "error", err)
	}
}

func serveMCP(cfg config.Config, logger *slog.Logger) {
	ctx := context.Background()
	pool := connect(ctx, cfg, logger)
	defer pool.Close()

	srv := mcp.NewServer("samplier", Version, logger)
	mcp.NewTools(pool, store.New(pool)).Register(srv)

	logger.Info("samplier MCP server ready on stdio", "version", Version, "tools", len(srv.Tools()))
	if err := srv.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		logger.Error("mcp server failed", "error", err)
		os.Exit(1)
	}
}
