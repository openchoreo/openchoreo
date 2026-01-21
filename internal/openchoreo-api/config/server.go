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
	// BindAddress is the address to bind the HTTP server to.
	BindAddress string `koanf:"bind_address"`
	// Port is the HTTP server port.
	Port int `koanf:"port"`
	// PublicURL is the externally accessible URL of this server.
	// Used for OAuth resource metadata and self-referential links.
	PublicURL string `koanf:"public_url"`
	// Timeouts defines HTTP server timeout settings.
	Timeouts TimeoutsConfig `koanf:"timeouts"`
	// TLS defines TLS/HTTPS settings.
	TLS TLSConfig `koanf:"tls"`
	// Middleware defines middleware configurations.
	Middleware MiddlewareConfig `koanf:"middleware"`
}

// TimeoutsConfig defines HTTP server timeout settings.
type TimeoutsConfig struct {
	// Read is the maximum duration for reading the entire request.
	Read time.Duration `koanf:"read"`
	// Write is the maximum duration before timing out writes of the response.
	Write time.Duration `koanf:"write"`
	// Idle is the maximum duration to wait for the next request.
	Idle time.Duration `koanf:"idle"`
	// Shutdown is the maximum duration to wait for active connections to close.
	Shutdown time.Duration `koanf:"shutdown"`
}

// TimeoutsDefaults returns the default timeout configuration.
func TimeoutsDefaults() TimeoutsConfig {
	return TimeoutsConfig{
		Read:     15 * time.Second,
		Write:    15 * time.Second,
		Idle:     60 * time.Second,
		Shutdown: 30 * time.Second,
	}
}

// TLSConfig defines TLS/HTTPS settings.
type TLSConfig struct {
	// Enabled enables TLS for the HTTP server.
	Enabled bool `koanf:"enabled"`
	// CertFile is the path to the TLS certificate file.
	CertFile string `koanf:"cert_file"`
	// KeyFile is the path to the TLS private key file.
	KeyFile string `koanf:"key_file"`
}

// TLSDefaults returns the default TLS configuration.
func TLSDefaults() TLSConfig {
	return TLSConfig{
		Enabled: false,
	}
}

// MiddlewareConfig defines server middleware configurations.
// Placeholder for future middleware settings (e.g., request_id).
type MiddlewareConfig struct {
	// Future: RequestID config, rate limiting, etc.
}

// MiddlewareDefaults returns the default middleware configuration.
func MiddlewareDefaults() MiddlewareConfig {
	return MiddlewareConfig{}
}

// Validate validates the middleware configuration.
func (c *MiddlewareConfig) Validate(path *config.Path) config.ValidationErrors {
	return nil // No validation needed for empty config
}

// ServerDefaults returns the default server configuration.
func ServerDefaults() ServerConfig {
	return ServerConfig{
		BindAddress: "0.0.0.0",
		Port:       8080,
		PublicURL:  "http://localhost:8080",
		Timeouts:   TimeoutsDefaults(),
		TLS:        TLSDefaults(),
		Middleware: MiddlewareDefaults(),
	}
}

// Validate validates the server configuration.
func (c *ServerConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeInRange(path.Child("port"), c.Port, 1, 65535); err != nil {
		errs = append(errs, err)
	}

	errs = append(errs, c.Timeouts.Validate(path.Child("timeouts"))...)
	errs = append(errs, c.TLS.Validate(path.Child("tls"))...)
	errs = append(errs, c.Middleware.Validate(path.Child("middleware"))...)

	return errs
}

// Validate validates the timeout configuration.
func (c *TimeoutsConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if err := config.MustBeNonNegative(path.Child("read"), c.Read); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("write"), c.Write); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("idle"), c.Idle); err != nil {
		errs = append(errs, err)
	}

	if err := config.MustBeNonNegative(path.Child("shutdown"), c.Shutdown); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// Validate validates the TLS configuration.
func (c *TLSConfig) Validate(path *config.Path) config.ValidationErrors {
	var errs config.ValidationErrors

	if !c.Enabled {
		return errs // skip validation if TLS is disabled
	}

	if c.CertFile == "" {
		errs = append(errs, config.Required(path.Child("cert_file")))
	}

	if c.KeyFile == "" {
		errs = append(errs, config.Required(path.Child("key_file")))
	}

	return errs
}

// ToServerConfig converts to the server library config.
func (c *ServerConfig) ToServerConfig() server.Config {
	return server.Config{
		Addr:            fmt.Sprintf("%s:%d", c.BindAddress, c.Port),
		ReadTimeout:     c.Timeouts.Read,
		WriteTimeout:    c.Timeouts.Write,
		IdleTimeout:     c.Timeouts.Idle,
		ShutdownTimeout: c.Timeouts.Shutdown,
		TLSEnabled:      c.TLS.Enabled,
		TLSCertFile:     c.TLS.CertFile,
		TLSKeyFile:      c.TLS.KeyFile,
	}
}
