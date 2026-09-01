// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package config loads the remote-connect SNI router configuration through the shared
// OpenChoreo config framework, so the router honors the same override chain as every
// other component: struct defaults, then a YAML file, then environment variables, then
// explicitly-set CLI flags.
package config

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	remoteagentrouter "github.com/openchoreo/openchoreo/internal/remote-agent-router"
)

// EnvPrefix is the environment-variable prefix. Nesting uses a double underscore:
// OC_REMOTE_AGENT_ROUTER__ROUTER__LISTEN_ADDR -> router.listen_addr.
const EnvPrefix = "OC_REMOTE_AGENT_ROUTER"

// Config is the top-level configuration for remote-agent-router.
type Config struct {
	// Router defines the SNI router's listener and agent-discovery settings.
	Router RouterConfig `koanf:"router"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// RouterConfig configures the SNI router.
type RouterConfig struct {
	// ListenAddr is the L4 address tunnel connections are accepted on.
	ListenAddr string `koanf:"listen_addr"`
	// LabelSelector identifies the remote-agent Services the router routes to.
	LabelSelector string `koanf:"label_selector"`
	// SNIAnnotationKey is the Service annotation holding each remote-agent's SNI host.
	SNIAnnotationKey string `koanf:"sni_annotation_key"`
	// RefreshInterval is how often the SNI -> backend map is refreshed.
	RefreshInterval time.Duration `koanf:"refresh_interval"`
	// DialTimeout bounds dialing a remote-agent backend.
	DialTimeout time.Duration `koanf:"dial_timeout"`
}

// LoggingConfig defines logging settings.
type LoggingConfig struct {
	// Level is the minimum log level (debug, info, warn, error).
	Level string `koanf:"level"`
	// Format is the log output format (json, text).
	Format string `koanf:"format"`
	// AddSource includes source file and line number in log entries.
	AddSource bool `koanf:"add_source"`
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		Router:  RouterDefaults(),
		Logging: LoggingDefaults(),
	}
}

// RouterDefaults returns the default router configuration.
func RouterDefaults() RouterConfig {
	return RouterConfig{
		ListenAddr:       ":8443",
		LabelSelector:    remoteagentrouter.DefaultLabelSelector,
		SNIAnnotationKey: remoteagentrouter.DefaultSNIAnnotationKey,
		RefreshInterval:  remoteagentrouter.DefaultRefreshInterval,
		DialTimeout:      remoteagentrouter.DefaultDialTimeout,
	}
}

// LoggingDefaults returns the default logging configuration.
func LoggingDefaults() LoggingConfig {
	return LoggingConfig{Level: "info", Format: "json"}
}

// flagMappings maps CLI flag names to config paths.
var flagMappings = map[string]string{
	"listen":           "router.listen_addr",
	"label-selector":   "router.label_selector",
	"sni-annotation":   "router.sni_annotation_key",
	"refresh-interval": "router.refresh_interval",
	"dial-timeout":     "router.dial_timeout",
	"log-level":        "logging.level",
	"log-format":       "logging.format",
}

// NewLoader creates a configuration loader with all sources loaded.
// Loading priority (highest to lowest):
//  1. CLI flags (only if explicitly set)
//  2. Environment variables (OC_REMOTE_AGENT_ROUTER__ROUTER__LISTEN_ADDR -> router.listen_addr)
//  3. Config file (YAML)
//  4. Struct defaults
//
// If configPath is empty, no config file is loaded.
// If flags is nil, no flag overrides are applied.
func NewLoader(configPath string, flags *pflag.FlagSet) (*coreconfig.Loader, error) {
	loader := coreconfig.NewLoader(EnvPrefix)

	if err := loader.LoadWithDefaults(Defaults(), configPath); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if flags != nil {
		if err := loader.LoadFlags(flags, flagMappings); err != nil {
			return nil, fmt.Errorf("failed to load flags: %w", err)
		}
	}

	return loader, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	var errs coreconfig.ValidationErrors

	errs = append(errs, c.Router.Validate(coreconfig.NewPath("router"))...)
	errs = append(errs, c.Logging.Validate(coreconfig.NewPath("logging"))...)

	return errs.OrNil()
}

// Validate validates the router configuration.
func (c *RouterConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	if err := coreconfig.MustNotBeEmpty(path.Child("listen_addr"), c.ListenAddr); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustNotBeEmpty(path.Child("sni_annotation_key"), c.SNIAnnotationKey); err != nil {
		errs = append(errs, err)
	}
	// A non-positive interval panics time.NewTicker in the discovery loop, and a
	// non-positive dial timeout expires every backend dial. Reject at startup rather
	// than crashing or dropping every connection.
	if err := coreconfig.MustBeGreaterThan(path.Child("refresh_interval"), c.RefreshInterval, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("dial_timeout"), c.DialTimeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// Validate validates the logging configuration.
func (c *LoggingConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	if err := coreconfig.MustBeOneOf(path.Child("level"), c.Level, []string{"debug", "info", "warn", "error"}); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeOneOf(path.Child("format"), c.Format, []string{"json", "text"}); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// ToLoggingConfig converts to the logging library config.
func (c *LoggingConfig) ToLoggingConfig() logging.Config {
	return logging.Config{Level: c.Level, Format: c.Format, AddSource: c.AddSource}
}

// ToRouterConfig maps the loaded configuration onto the router's runtime config.
func (c *Config) ToRouterConfig() remoteagentrouter.Config {
	return remoteagentrouter.Config{
		ListenAddr:       c.Router.ListenAddr,
		LabelSelector:    c.Router.LabelSelector,
		SNIAnnotationKey: c.Router.SNIAnnotationKey,
		RefreshInterval:  c.Router.RefreshInterval,
		DialTimeout:      c.Router.DialTimeout,
	}
}
