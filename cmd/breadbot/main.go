// Command breadbot runs the Discord bread-detection bot and, alongside it, a
// thin read-only HTTP stats API in a single process.
//
// Startup order (per the migration plan): load config -> open DB + create
// tables -> ensure download dirs -> start HTTP server (goroutine) -> open the
// discordgo session. It blocks until SIGINT/SIGTERM, then shuts everything down
// gracefully.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hawawa4/breadbotdiscord/internal/bot"
	"github.com/hawawa4/breadbotdiscord/internal/config"
	"github.com/hawawa4/breadbotdiscord/internal/db"
	"github.com/hawawa4/breadbotdiscord/internal/httpserver"
	"github.com/hawawa4/breadbotdiscord/internal/inference"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	// Load .env for local dev (real environments set vars directly).
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	// Open DB (creates schema if missing).
	database, err := db.Open(cfg.DBDataPath)
	if err != nil {
		return err
	}
	defer database.Close()

	// Ensure runtime directories exist.
	for _, dir := range []string{
		cfg.DownloadsPath,
		filepath.Join(cfg.DownloadsPath, "plots"),
		filepath.Join(cfg.DownloadsPath, "predictions"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	inf := inference.NewClient(cfg.InferenceServiceURL)

	discordBot, err := bot.New(cfg, database, inf)
	if err != nil {
		return err
	}

	// Start the read-only HTTP server in a goroutine.
	srv := httpserver.New(cfg.AdminAPIAddr, database, discordBot, cfg.AdminAPIToken)
	go func() {
		slog.Info("http server listening", "addr", cfg.AdminAPIAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
		}
	}()

	// Open the Discord session (connects the gateway).
	if err := discordBot.Open(); err != nil {
		return err
	}
	slog.Info("breadbot started; press Ctrl+C to stop")

	// Catch up on messages missed while offline. Runs in the background so it
	// never delays shutdown; it reuses the normal message pipeline.
	go discordBot.CatchUp()

	// Block until interrupted.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")

	// Graceful shutdown: close session + HTTP server (DB closes via defer).
	if err := discordBot.Close(); err != nil {
		slog.Error("close discord session", "err", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown http server", "err", err)
	}
	return nil
}
