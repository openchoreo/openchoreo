package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/exp/slog"

	k8s "github.com/openchoreo/openchoreo/internal/openchoreo-api/clients"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/handlers"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

var (
	port = flag.Int("port", 8080, "port http server runs on")
)

func main() {
	flag.Parse()

	slogHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	baseLogger := slog.New(slogHandler)
	slog.SetDefault(baseLogger)

	// Create shutdown context
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	k8sClient, err := k8s.NewK8sClient()
	if err != nil {
		baseLogger.Error("Failed to initialize Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize services
	services := services.NewServices(k8sClient, baseLogger)

	// Initialize HTTP handlers
	handler := handlers.New(services, baseLogger.With("component", "handlers"))

	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(*port),
		Handler:      handler.Routes(),
		ReadTimeout:  15 * time.Second, // TODO: Make these configurable
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server
	go func() {
		baseLogger.Info("OpenChoreo API server listening on", slog.String("address", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			baseLogger.Error("Server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		baseLogger.Error("Server shutdown error", slog.Any("error", err))
	}

	baseLogger.Info("Server stopped gracefully")
}
