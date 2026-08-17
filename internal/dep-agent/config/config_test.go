// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFlags mirrors the flag set cmd/dep-agent registers, so the mappings under test are
// the ones the binary actually uses.
func testFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("dep-agent", pflag.ContinueOnError)
	flags.String("listen", AgentDefaults().ListenAddr, "")
	flags.String("namespace", AgentDefaults().Namespace, "")
	flags.Int("max-streams-per-session", AgentDefaults().MaxStreamsPerSession, "")
	flags.Duration("dial-timeout", AgentDefaults().DialTimeout, "")
	flags.String("authorize-url", "", "")
	flags.Bool("authorize-insecure", false, "")
	flags.String("log-level", LoggingDefaults().Level, "")
	require.NoError(t, flags.Parse(args))
	return flags
}

func load(t *testing.T, configPath string, flags *pflag.FlagSet) Config {
	t.Helper()
	loader, err := NewLoader(configPath, flags)
	require.NoError(t, err)
	var cfg Config
	require.NoError(t, loader.Unmarshal("", &cfg))
	return cfg
}

func TestDefaults(t *testing.T) {
	cfg := load(t, "", nil)

	assert.Equal(t, ":8443", cfg.Agent.ListenAddr)
	assert.Equal(t, "/certs/tls.crt", cfg.Agent.TLSCertPath)
	assert.Equal(t, 256, cfg.Agent.MaxStreamsPerSession)
	assert.Equal(t, 10*time.Second, cfg.Agent.DialTimeout)
	assert.Equal(t, time.Minute, cfg.Heartbeat.Interval)
	assert.Equal(t, "info", cfg.Logging.Level)
}

// TestFlagsDecodeToTypedFields: the loader stores flag values as strings, so bool, int
// and duration flags must survive the round trip into their typed fields. The
// provisioner passes --authorize-insecure as a bare flag.
func TestFlagsDecodeToTypedFields(t *testing.T) {
	cfg := load(t, "", testFlags(t,
		"--authorize-insecure",
		"--max-streams-per-session=32",
		"--dial-timeout=45s",
		"--authorize-url=https://cp/authorize",
	))

	assert.True(t, cfg.Authorize.InsecureSkipVerify)
	assert.Equal(t, 32, cfg.Agent.MaxStreamsPerSession)
	assert.Equal(t, 45*time.Second, cfg.Agent.DialTimeout)
	assert.Equal(t, "https://cp/authorize", cfg.Authorize.URL)
}

// TestOverridePrecedence: defaults < file < env < explicitly-set flags.
func TestOverridePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
agent:
  listen_addr: ":6443"
  max_streams_per_session: 8
authorize:
  url: "https://from-file/authorize"
logging:
  level: warn
`), 0o600))

	t.Setenv("OC_DEP_AGENT__AGENT__LISTEN_ADDR", ":7443")
	t.Setenv("OC_DEP_AGENT__LOGGING__LEVEL", "debug")

	cfg := load(t, path, testFlags(t, "--log-level=error"))

	assert.Equal(t, ":7443", cfg.Agent.ListenAddr, "env must beat the file")
	assert.Equal(t, "error", cfg.Logging.Level, "an explicit flag must beat env")
	assert.Equal(t, 8, cfg.Agent.MaxStreamsPerSession, "the file must beat defaults")
	assert.Equal(t, "https://from-file/authorize", cfg.Authorize.URL)
}

// TestUnsetFlagsDoNotOverride: a flag left at its default must not mask a file value.
func TestUnsetFlagsDoNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("logging:\n  level: warn\n"), 0o600))

	cfg := load(t, path, testFlags(t))

	assert.Equal(t, "warn", cfg.Logging.Level)
}

// TestNamespaceDefaultsToDownwardAPI: the control plane injects POD_NAMESPACE into every
// provisioned agent Pod, and heartbeats are silently disabled without it.
func TestNamespaceDefaultsToDownwardAPI(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "dp-doclet-development")

	assert.Equal(t, "dp-doclet-development", load(t, "", nil).Agent.Namespace)

	cfg := load(t, "", testFlags(t, "--namespace=explicit"))
	assert.Equal(t, "explicit", cfg.Agent.Namespace, "--namespace must beat the downward-API default")
}

func TestConfigFileNotFound(t *testing.T) {
	_, err := NewLoader(filepath.Join(t.TempDir(), "missing.yaml"), nil)
	assert.Error(t, err)
}

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Config)
		wantErr string
	}{
		"valid":                 {mutate: func(*Config) {}},
		"missing authorize url": {mutate: func(c *Config) { c.Authorize.URL = "" }, wantErr: "authorize.url"},
		"empty listen addr":     {mutate: func(c *Config) { c.Agent.ListenAddr = "" }, wantErr: "agent.listen_addr"},
		"empty tls cert":        {mutate: func(c *Config) { c.Agent.TLSCertPath = "" }, wantErr: "agent.tls_cert_path"},
		"negative max streams":  {mutate: func(c *Config) { c.Agent.MaxStreamsPerSession = -1 }, wantErr: "agent.max_streams_per_session"},
		"zero dial timeout":     {mutate: func(c *Config) { c.Agent.DialTimeout = 0 }, wantErr: "agent.dial_timeout"},
		"bad log level":         {mutate: func(c *Config) { c.Logging.Level = "trace" }, wantErr: "logging.level"},
		// A zero interval panics the heartbeat ticker, but only once heartbeats are on.
		"zero heartbeat interval without url": {mutate: func(c *Config) { c.Heartbeat.Interval = 0 }},
		"zero heartbeat interval with url": {
			mutate:  func(c *Config) { c.Heartbeat.URL, c.Heartbeat.Interval = "https://cp/heartbeat", 0 },
			wantErr: "heartbeat.interval",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Authorize.URL = "https://cp/authorize"
			tt.mutate(&cfg)

			err := cfg.Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestToAgentConfig checks every loaded field reaches the runtime config.
func TestToAgentConfig(t *testing.T) {
	cfg := Defaults()
	cfg.Agent.ListenAddr = ":9443"
	cfg.Agent.Namespace = "dp-doclet-development"
	cfg.Agent.MaxStreamsPerSession = 16
	cfg.Authorize.URL = "https://cp/authorize"
	cfg.Authorize.CABundlePath = "/ca/ca.crt"
	cfg.Authorize.InsecureSkipVerify = true
	cfg.Heartbeat.URL = "https://cp/heartbeat"

	got := cfg.ToAgentConfig()

	assert.Equal(t, ":9443", got.ListenAddr)
	assert.Equal(t, "/certs/tls.crt", got.TLSCertPath)
	assert.Equal(t, "/certs/tls.key", got.TLSKeyPath)
	assert.Equal(t, "https://cp/authorize", got.AuthorizeURL)
	assert.Equal(t, "/ca/ca.crt", got.AuthorizeCABundlePath)
	assert.True(t, got.AuthorizeInsecureSkipVerify)
	assert.Equal(t, "https://cp/heartbeat", got.HeartbeatURL)
	assert.Equal(t, time.Minute, got.HeartbeatInterval)
	assert.Equal(t, "dp-doclet-development", got.Namespace)
	assert.Equal(t, 10*time.Second, got.HandshakeTimeout)
	assert.Equal(t, 10*time.Second, got.StreamOpenTimeout)
	assert.Equal(t, 10*time.Second, got.AuthorizeTimeout)
	assert.Equal(t, 10*time.Second, got.DialTimeout)
	assert.Equal(t, 16, got.MaxStreamsPerSession)
}
