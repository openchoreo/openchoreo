// Copyright 2026 The OpenChoreo Authors
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
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openchoreo/openchoreo/internal/eventforwarder"
	"github.com/openchoreo/openchoreo/internal/eventforwarder/config"
	"github.com/openchoreo/openchoreo/internal/eventforwarder/dispatcher"
	"github.com/openchoreo/openchoreo/internal/logging"
	"github.com/openchoreo/openchoreo/internal/version"
)

func main() {
	configPath := flag.String("config", "/etc/openchoreo/config.yaml", "Path to configuration file")
	flag.Parse()

	// Bootstrap logger for pre-configuration errors. Component name is
	// baked into the binary by the build's -X linker flag (see
	// make/golang.mk → GO_LDFLAGS_BUILD_DATA).
	bootstrapLogger := logging.Bootstrap(version.Get().Name)

	cfg, err := config.Load(*configPath)
	if err != nil {
		bootstrapLogger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set up runtime logger from configuration. Same shape as every
	// other OpenChoreo binary so log fields and formats stay consistent
	// across components.
	logger := logging.NewWithComponent(cfg.Logging.ToLoggingConfig(), version.Get().Name)
	logger.Info("Starting", version.GetLogKeyValues()...)
	logger.Info("Configuration loaded", "logLevel", cfg.Logging.Level)

	// Resolve the Kubernetes REST config via controller-runtime so the
	// binary works both in-cluster (uses the pod's service account
	// token) and locally (falls back to KUBECONFIG / ~/.kube/config).
	// `rest.InClusterConfig()` would have forced an in-cluster-only
	// flow, breaking local testing.
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		logger.Error("Failed to obtain Kubernetes REST config", slog.Any("error", err))
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		logger.Error("Failed to create dynamic Kubernetes client", slog.Any("error", err))
		os.Exit(1)
	}

	// Initialize dispatcher
	d := dispatcher.New(cfg.Webhooks, logger.With("component", "dispatcher"))

	// Initialize event-forwarder
	f := eventforwarder.New(dynamicClient, d, logger.With("component", "event-forwarder"))

	// Initialize health server
	healthSrv := eventforwarder.NewHealthServer(logger.With("component", "health"))

	// Start health server (note: /ready stays NotReady until Forwarder
	// signals onReady below, so rolling-update traffic isn't routed to
	// this pod before its informer caches have synced).
	go func() {
		if err := healthSrv.ListenAndServe(cfg.Server.Port); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Health server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the dispatcher worker pool before any events can be queued
	// — Forwarder.Start triggers an initial replay of "create" events
	// for every existing resource as informer caches sync, so workers
	// must be running by then.
	d.Start(ctx)

	logger.Info("Starting event-forwarder")
	// SetReady fires only after every informer cache has finished its
	// initial list — see Forwarder.Start for the synchronization point.
	if err := f.Start(ctx, healthSrv.SetReady); err != nil {
		logger.Error("Event-forwarder exited with error", "error", err)
		os.Exit(1)
	}

	logger.Info("Event-forwarder shutdown complete")
}
