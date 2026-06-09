// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// HealthServer provides /health and /ready endpoints.
type HealthServer struct {
	logger *slog.Logger
	ready  atomic.Bool
}

// NewHealthServer creates a new HealthServer.
func NewHealthServer(logger *slog.Logger) *HealthServer {
	return &HealthServer{
		logger: logger,
	}
}

// SetReady marks the server as ready to receive traffic.
func (s *HealthServer) SetReady() {
	s.ready.Store(true)
}

// Handler returns an http.Handler with /health and /ready routes.
func (s *HealthServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.healthHandler)
	mux.HandleFunc("GET /ready", s.readyHandler)
	return mux
}

// ListenAndServe starts the health server on the given port.
func (s *HealthServer) ListenAndServe(port int) error {
	addr := fmt.Sprintf(":%d", port)
	s.logger.Info("Starting health server", "address", addr)
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

func (s *HealthServer) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		s.logger.Warn("Failed to encode health response", "error", err)
	}
}

func (s *HealthServer) readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var payload map[string]string
	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		payload = map[string]string{"status": "ready"}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		payload = map[string]string{"status": "not ready"}
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		s.logger.Warn("Failed to encode readiness response", "error", err)
	}
}
