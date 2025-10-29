// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/openchoreo/openchoreo/internal/security-scanner/api"
	"github.com/openchoreo/openchoreo/internal/security-scanner/controller"
	"github.com/openchoreo/openchoreo/internal/security-scanner/db"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Starting security-scanner")

	dbBackend := os.Getenv("DB_BACKEND")
	if dbBackend == "" {
		dbBackend = "sqlite"
	}

	dbDSN := os.Getenv("DB_DSN")
	if dbDSN == "" {
		dbDSN = "/tmp/security-scanner.db"
	}

	dbConn, err := db.InitDB(db.Config{
		Backend: db.DBBackend(dbBackend),
		DSN:     dbDSN,
	})
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer dbConn.Close()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	if err != nil {
		logger.Error("Failed to create controller manager", "error", err)
		os.Exit(1)
	}

	podReconciler := &controller.PodReconciler{
		Client:  mgr.GetClient(),
		Scheme:  mgr.GetScheme(),
		Queries: dbConn.Querier(),
	}

	if err := podReconciler.SetupWithManager(mgr); err != nil {
		logger.Error("Failed to setup pod controller", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("Starting HTTP server", "address", ":8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		logger.Info("Starting controller manager")
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			logger.Error("Controller manager error", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	logger.Info("Shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", "error", err)
	}

	logger.Info("Shutdown complete")
}
