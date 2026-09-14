// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package config loads the remote-agent configuration through the shared OpenChoreo
// config framework, so the agent honors the same override chain as every other
// component: struct defaults, then a YAML file, then environment variables, then
// explicitly-set CLI flags.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
	"github.com/openchoreo/openchoreo/internal/logging"
	remoteagent "github.com/openchoreo/openchoreo/internal/remote-agent"
)

// EnvPrefix is the environment-variable prefix. Nesting uses a double underscore:
// OC_REMOTE_AGENT__AGENT__LISTEN_ADDR -> agent.listen_addr.
const EnvPrefix = "OC_REMOTE_AGENT"

// namespaceEnv is the downward-API variable the control plane injects into every
// provisioned remote-agent Pod. It seeds agent.namespace as the lowest-priority default,
// so a config file, a prefixed env var, or --namespace still override it.
const namespaceEnv = "POD_NAMESPACE"

// Config is the top-level configuration for remote-agent.
type Config struct {
	// Agent defines the tunnel listener, its TLS material, and per-stream limits.
	Agent AgentConfig `koanf:"agent"`
	// Authorize defines the control-plane authorize endpoint called per stream.
	Authorize AuthorizeConfig `koanf:"authorize"`
	// Heartbeat defines the liveness refresh that keeps a busy agent from being reaped.
	Heartbeat HeartbeatConfig `koanf:"heartbeat"`
	// Logging defines logging settings.
	Logging LoggingConfig `koanf:"logging"`
}

// AgentConfig configures the tunnel listener.
type AgentConfig struct {
	// ListenAddr is the TCP address the TLS tunnel listener binds to.
	ListenAddr string `koanf:"listen_addr"`
	// TLSCertPath / TLSKeyPath are the server certificate and key presented to occ.
	TLSCertPath string `koanf:"tls_cert_path"`
	TLSKeyPath  string `koanf:"tls_key_path"`
	// Namespace is the agent's own data-plane namespace, sent in heartbeats so the
	// control plane refreshes the right agent. Empty disables heartbeats.
	Namespace string `koanf:"namespace"`
	// MaxStreamsPerSession caps concurrent streams on one tunnel (0 = unlimited).
	MaxStreamsPerSession int `koanf:"max_streams_per_session"`
	// HandshakeTimeout bounds the Hello/HelloResult exchange.
	HandshakeTimeout time.Duration `koanf:"handshake_timeout"`
	// StreamOpenTimeout bounds how long occ may take to send StreamOpen.
	StreamOpenTimeout time.Duration `koanf:"stream_open_timeout"`
	// DialTimeout bounds dialing an upstream dependency target.
	DialTimeout time.Duration `koanf:"dial_timeout"`
	// ReadTimeout bounds a single Secret/ConfigMap read against the Kubernetes API.
	ReadTimeout time.Duration `koanf:"read_timeout"`
}

// AuthorizeConfig configures the per-stream authorization call to the control plane.
type AuthorizeConfig struct {
	// URL is the control-plane authorize endpoint (POST) called per stream.
	URL string `koanf:"url"`
	// CABundlePath, when set, pins the CA trusted when calling the control plane.
	// Empty uses the system roots.
	CABundlePath string `koanf:"ca_bundle_path"`
	// InsecureSkipVerify disables TLS verification of the control plane
	// (development only).
	InsecureSkipVerify bool `koanf:"insecure_skip_verify"`
	// Timeout bounds a single authorize call.
	Timeout time.Duration `koanf:"timeout"`
}

// HeartbeatConfig configures the liveness refresh sent while sessions are live.
type HeartbeatConfig struct {
	// URL is the control-plane heartbeat endpoint (POST). Empty disables heartbeats.
	URL string `koanf:"url"`
	// Interval is how often liveness is refreshed while sessions are live.
	Interval time.Duration `koanf:"interval"`
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
		Agent:     AgentDefaults(),
		Authorize: AuthorizeDefaults(),
		Heartbeat: HeartbeatDefaults(),
		Logging:   LoggingDefaults(),
	}
}

// AgentDefaults returns the default agent configuration.
func AgentDefaults() AgentConfig {
	return AgentConfig{
		ListenAddr:           ":8443",
		TLSCertPath:          "/certs/tls.crt",
		TLSKeyPath:           "/certs/tls.key",
		Namespace:            os.Getenv(namespaceEnv),
		MaxStreamsPerSession: 256,
		HandshakeTimeout:     remoteagent.DefaultHandshakeTimeout,
		StreamOpenTimeout:    remoteagent.DefaultStreamOpenTimeout,
		DialTimeout:          remoteagent.DefaultDialTimeout,
		ReadTimeout:          remoteagent.DefaultReadTimeout,
	}
}

// AuthorizeDefaults returns the default authorize configuration.
func AuthorizeDefaults() AuthorizeConfig {
	return AuthorizeConfig{Timeout: remoteagent.DefaultAuthorizeTimeout}
}

// HeartbeatDefaults returns the default heartbeat configuration.
func HeartbeatDefaults() HeartbeatConfig {
	return HeartbeatConfig{Interval: remoteagent.DefaultHeartbeatInterval}
}

// LoggingDefaults returns the default logging configuration.
func LoggingDefaults() LoggingConfig {
	return LoggingConfig{Level: "info", Format: "json"}
}

// flagMappings maps CLI flag names to config paths.
var flagMappings = map[string]string{
	"listen":                  "agent.listen_addr",
	"tls-cert":                "agent.tls_cert_path",
	"tls-key":                 "agent.tls_key_path",
	"namespace":               "agent.namespace",
	"max-streams-per-session": "agent.max_streams_per_session",
	"handshake-timeout":       "agent.handshake_timeout",
	"stream-open-timeout":     "agent.stream_open_timeout",
	"dial-timeout":            "agent.dial_timeout",
	"authorize-url":           "authorize.url",
	"authorize-ca":            "authorize.ca_bundle_path",
	"authorize-insecure":      "authorize.insecure_skip_verify",
	"authorize-timeout":       "authorize.timeout",
	"heartbeat-url":           "heartbeat.url",
	"heartbeat-interval":      "heartbeat.interval",
	"log-level":               "logging.level",
	"log-format":              "logging.format",
}

// NewLoader creates a configuration loader with all sources loaded.
// Loading priority (highest to lowest):
//  1. CLI flags (only if explicitly set)
//  2. Environment variables (OC_REMOTE_AGENT__AGENT__LISTEN_ADDR -> agent.listen_addr)
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

	errs = append(errs, c.Agent.Validate(coreconfig.NewPath("agent"))...)
	errs = append(errs, c.Authorize.Validate(coreconfig.NewPath("authorize"))...)
	errs = append(errs, c.Heartbeat.Validate(coreconfig.NewPath("heartbeat"))...)
	errs = append(errs, c.Logging.Validate(coreconfig.NewPath("logging"))...)

	return errs.OrNil()
}

// Validate validates the agent configuration.
func (c *AgentConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	if err := coreconfig.MustNotBeEmpty(path.Child("listen_addr"), c.ListenAddr); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustNotBeEmpty(path.Child("tls_cert_path"), c.TLSCertPath); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustNotBeEmpty(path.Child("tls_key_path"), c.TLSKeyPath); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeNonNegative(path.Child("max_streams_per_session"), c.MaxStreamsPerSession); err != nil {
		errs = append(errs, err)
	}
	// A non-positive timeout expires before the exchange it bounds can complete, so
	// every tunnel would be dropped. Reject at startup rather than serving nothing.
	if err := coreconfig.MustBeGreaterThan(path.Child("handshake_timeout"), c.HandshakeTimeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("stream_open_timeout"), c.StreamOpenTimeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("dial_timeout"), c.DialTimeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("read_timeout"), c.ReadTimeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// Validate validates the authorize configuration.
func (c *AuthorizeConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	// Without an authorize endpoint the agent can resolve no stream at all.
	if c.URL == "" {
		errs = append(errs, coreconfig.Required(path.Child("url")))
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("timeout"), c.Timeout, time.Duration(0)); err != nil {
		errs = append(errs, err)
	}

	return errs
}

// Validate validates the heartbeat configuration.
func (c *HeartbeatConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors

	// Heartbeats are optional, but a non-positive interval panics the ticker driving
	// them, so it is only checked once an endpoint is configured.
	if c.URL != "" {
		if err := coreconfig.MustBeGreaterThan(path.Child("interval"), c.Interval, time.Duration(0)); err != nil {
			errs = append(errs, err)
		}
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

// ToAgentConfig maps the loaded configuration onto the agent's runtime config.
func (c *Config) ToAgentConfig() remoteagent.Config {
	return remoteagent.Config{
		ListenAddr:                  c.Agent.ListenAddr,
		TLSCertPath:                 c.Agent.TLSCertPath,
		TLSKeyPath:                  c.Agent.TLSKeyPath,
		AuthorizeURL:                c.Authorize.URL,
		AuthorizeCABundlePath:       c.Authorize.CABundlePath,
		AuthorizeInsecureSkipVerify: c.Authorize.InsecureSkipVerify,
		HeartbeatURL:                c.Heartbeat.URL,
		HeartbeatInterval:           c.Heartbeat.Interval,
		Namespace:                   c.Agent.Namespace,
		HandshakeTimeout:            c.Agent.HandshakeTimeout,
		StreamOpenTimeout:           c.Agent.StreamOpenTimeout,
		AuthorizeTimeout:            c.Authorize.Timeout,
		DialTimeout:                 c.Agent.DialTimeout,
		ReadTimeout:                 c.Agent.ReadTimeout,
		MaxStreamsPerSession:        c.Agent.MaxStreamsPerSession,
	}
}
