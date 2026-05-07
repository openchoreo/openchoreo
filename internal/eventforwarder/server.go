// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package eventforwarder

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
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
	return http.ListenAndServe(addr, s.Handler())
}

func (s *HealthServer) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *HealthServer) readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.ready.Load() {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
	}
}
