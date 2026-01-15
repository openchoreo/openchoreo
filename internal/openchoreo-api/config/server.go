// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"time"

	"github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/server"
)

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	// Port is the HTTP server port.
	Port int `koanf:"port"`
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `koanf:"read_timeout"`
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `koanf:"write_timeout"`
	// IdleTimeout is the maximum duration to wait for the next request.
	IdleTimeout time.Duration `koanf:"idle_timeout"`
	// ShutdownTimeout is the maximum duration to wait for active connections to close.
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
	// Middleware defines middleware configurations.
	Middleware MiddlewareConfig `koanf:"middleware"`
}

// ServerDefaults returns the default server configuration.
func ServerDefaults() ServerConfig {
	return ServerConfig{
		Port:            8080,
		ReadTimeout:     15 * time.Second,
		WriteTimeout:    15 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		Middleware:      MiddlewareDefaults(),
	}
}

// Validate validates the server configuration.
func (c *ServerConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeInRange(path.Child("port"), c.Port, 1, 65535); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("read_timeout"), c.ReadTimeout); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("write_timeout"), c.WriteTimeout); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("idle_timeout"), c.IdleTimeout); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("shutdown_timeout"), c.ShutdownTimeout); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, c.Middleware.Validate(path.Child("middleware"))...)

	return errs
}

// ToServerConfig converts to the server library config.
func (c *ServerConfig) ToServerConfig() server.Config {
	return server.Config{
		Addr:            fmt.Sprintf(":%d", c.Port),
		ReadTimeout:     c.ReadTimeout,
		WriteTimeout:    c.WriteTimeout,
		IdleTimeout:     c.IdleTimeout,
		ShutdownTimeout: c.ShutdownTimeout,
	}
}
