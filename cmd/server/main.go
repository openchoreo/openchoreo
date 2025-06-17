// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/openchoreo/openchoreo/server"
	"github.com/openchoreo/openchoreo/server/pkg/logging"
)

var shutdownTimeout = 5 * time.Second

var port = flag.Int("port", 8080, "port http server runs on")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := logging.NewLogger()
	srv := server.NewServer(logger, server.ServerOptions{
		Port: *port,
	})

	go func() {
		logger.Info("Starting server on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error", "error", err)
		}
	}()

	<-ctx.Done()
	logger.Info("Received shutdown signal, gracefully shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Error during server shutdown", "error", err)
	} else {
		logger.Info("Server gracefully shut down")
	}
}
