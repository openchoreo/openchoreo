// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// DefaultShutdownTimeout is the default timeout for graceful shutdown.
const DefaultShutdownTimeout = 30 * time.Second

// Config holds the configuration for an HTTP server.
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	TLSEnabled      bool
	TLSCertFile     string
	TLSKeyFile      string
}

// Server wraps an HTTP server with lifecycle management.
type Server struct {
	httpServer      *http.Server
	logger          *slog.Logger
	shutdownTimeout time.Duration
	tlsEnabled      bool
	tlsCertFile     string
	tlsKeyFile      string
}

// New creates a new Server with the given configuration and handler.
// If ShutdownTimeout is not set, DefaultShutdownTimeout is used.
func New(cfg Config, handler http.Handler, logger *slog.Logger) *Server {
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = DefaultShutdownTimeout
	}

	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
		logger:          logger.With("module", "server"),
		shutdownTimeout: shutdownTimeout,
		tlsEnabled:      cfg.TLSEnabled,
		tlsCertFile:     cfg.TLSCertFile,
		tlsKeyFile:      cfg.TLSKeyFile,
	}
}

// Run starts the server and blocks until the context is cancelled.
// It handles graceful shutdown when the context is done.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("server starting", "addr", s.httpServer.Addr, "tls", s.tlsEnabled)
		var err error
		if s.tlsEnabled {
			err = s.httpServer.ListenAndServeTLS(s.tlsCertFile, s.tlsKeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
