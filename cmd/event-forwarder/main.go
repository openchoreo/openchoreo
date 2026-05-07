// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/openchoreo/openchoreo/internal/eventforwarder"
	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
	"github.com/openchoreo/openchoreo/internal/eventforwarder/dispatcher"
)

func main() {
	configPath := flag.String("config", "/etc/openchoreo/config.yaml", "Path to configuration file")
	flag.Parse()

	// Bootstrap logger
	bootstrapLogger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		bootstrapLogger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg.Logging.Level)
	logger.Info("Configuration loaded successfully", "logLevel", cfg.Logging.Level)

	// Initialize Kubernetes client
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("Failed to create in-cluster config", "error", err)
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		logger.Error("Failed to create dynamic Kubernetes client", "error", err)
		os.Exit(1)
	}

	// Initialize dispatcher
	d := dispatcher.New(cfg.Webhooks, logger.With("component", "dispatcher"))

	// Initialize event-forwarder
	f := eventforwarder.New(dynamicClient, d, logger.With("component", "event-forwarder"))

	// Initialize health server
	healthSrv := eventforwarder.NewHealthServer(logger.With("component", "health"))

	// Start health server
	go func() {
		if err := healthSrv.ListenAndServe(cfg.Server.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Health server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start event-forwarder (blocks until context is cancelled)
	logger.Info("Starting CRD event-forwarder")
	healthSrv.SetReady()

	if err := f.Start(ctx); err != nil {
		logger.Error("Event-forwarder exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("Event-forwarder shutdown complete")
}

func initLogger(level string) *slog.Logger {
	var logLevel slog.Level

	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: logLevel}

	var handler slog.Handler
	if level == "debug" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
