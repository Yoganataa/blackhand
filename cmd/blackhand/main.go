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

	"github.com/Yoganataa/blackhand/internal/config"
	"github.com/Yoganataa/blackhand/internal/httpapi"
	"github.com/Yoganataa/blackhand/internal/storage"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	store, err := storage.Open(cfg.DataDir)
	if err != nil {
		logger.Error("storage initialization failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	server := &http.Server{
		Addr: cfg.Address(), Handler: httpapi.New(cfg, store, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		// No global WriteTimeout: long-running AI streams must remain open.
		IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("blackhand starting", "address", server.Addr, "data_dir", cfg.DataDir)
		errCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil { logger.Warn("graceful shutdown failed", "error", err) }
	}
}
