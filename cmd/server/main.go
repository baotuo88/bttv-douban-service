package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kerkerker-douban-service/internal/app"
	"kerkerker-douban-service/internal/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	// Setup logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	// Load configuration
	cfg := config.Load()
	log.Info().
		Str("port", cfg.Port).
		Str("mode", cfg.GinMode).
		Int("proxies", len(cfg.DoubanProxies)).
		Msg("🚀 Starting kerkerker-douban-service")

	application, err := app.New(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize application")
	}
	defer application.Close()

	// Create HTTP server with graceful shutdown support
	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:    addr,
		Handler: application.Router,
	}

	// Start server in a goroutine
	go func() {
		log.Info().Str("addr", addr).Msg("🌐 Server listening")
		log.Info().Str("admin", "http://localhost"+addr+"/admin").Msg("📊 Admin dashboard available")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("🛑 Shutting down server...")

	// Give outstanding requests a deadline for completion
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("👋 Server exited")
}
