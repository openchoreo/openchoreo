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

	remoteagentrouter "github.com/openchoreo/openchoreo/internal/remote-agent-router"
)

// testFlags mirrors the flag set cmd/remote-agent-router registers, so the mappings under
// test are the ones the binary actually uses.
func testFlags(t *testing.T, args ...string) *pflag.FlagSet {
	t.Helper()
	flags := pflag.NewFlagSet("remote-agent-router", pflag.ContinueOnError)
	flags.String("listen", RouterDefaults().ListenAddr, "")
	flags.String("label-selector", RouterDefaults().LabelSelector, "")
	flags.String("sni-annotation", RouterDefaults().SNIAnnotationKey, "")
	flags.Duration("refresh-interval", RouterDefaults().RefreshInterval, "")
	flags.Duration("dial-timeout", RouterDefaults().DialTimeout, "")
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

	assert.Equal(t, ":8443", cfg.Router.ListenAddr)
	assert.Equal(t, remoteagentrouter.DefaultLabelSelector, cfg.Router.LabelSelector)
	assert.Equal(t, remoteagentrouter.DefaultSNIAnnotationKey, cfg.Router.SNIAnnotationKey)
	assert.Equal(t, remoteagentrouter.DefaultRefreshInterval, cfg.Router.RefreshInterval)
	assert.Equal(t, remoteagentrouter.DefaultDialTimeout, cfg.Router.DialTimeout)
	assert.Equal(t, "info", cfg.Logging.Level)
}

// TestFlagsDecodeToTypedFields: the loader stores flag values as strings, so duration
// flags must survive the round trip into their typed fields.
func TestFlagsDecodeToTypedFields(t *testing.T) {
	cfg := load(t, "", testFlags(t, "--refresh-interval=30s", "--dial-timeout=2s", "--listen=:9443"))

	assert.Equal(t, 30*time.Second, cfg.Router.RefreshInterval)
	assert.Equal(t, 2*time.Second, cfg.Router.DialTimeout)
	assert.Equal(t, ":9443", cfg.Router.ListenAddr)
}

// TestOverridePrecedence: defaults < file < env < explicitly-set flags.
func TestOverridePrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
router:
  listen_addr: ":6443"
  refresh_interval: 15s
logging:
  level: warn
`), 0o600))

	t.Setenv("OC_REMOTE_AGENT_ROUTER__ROUTER__LISTEN_ADDR", ":7443")
	t.Setenv("OC_REMOTE_AGENT_ROUTER__LOGGING__LEVEL", "debug")

	cfg := load(t, path, testFlags(t, "--log-level=error"))

	assert.Equal(t, ":7443", cfg.Router.ListenAddr, "env must beat the file")
	assert.Equal(t, "error", cfg.Logging.Level, "an explicit flag must beat env")
	assert.Equal(t, 15*time.Second, cfg.Router.RefreshInterval, "the file must beat defaults")
}

// TestUnsetFlagsDoNotOverride: a flag left at its default must not mask a file value.
func TestUnsetFlagsDoNotOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("router:\n  listen_addr: \":6443\"\n"), 0o600))

	cfg := load(t, path, testFlags(t))

	assert.Equal(t, ":6443", cfg.Router.ListenAddr)
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
		"empty listen addr":     {mutate: func(c *Config) { c.Router.ListenAddr = "" }, wantErr: "router.listen_addr"},
		"empty sni annotation":  {mutate: func(c *Config) { c.Router.SNIAnnotationKey = "" }, wantErr: "router.sni_annotation_key"},
		"zero refresh interval": {mutate: func(c *Config) { c.Router.RefreshInterval = 0 }, wantErr: "router.refresh_interval"},
		"negative refresh interval": {mutate: func(c *Config) { c.Router.RefreshInterval = -time.Second },
			wantErr: "router.refresh_interval"},
		"zero dial timeout": {mutate: func(c *Config) { c.Router.DialTimeout = 0 }, wantErr: "router.dial_timeout"},
		"bad log level":     {mutate: func(c *Config) { c.Logging.Level = "trace" }, wantErr: "logging.level"},
		"bad log format":    {mutate: func(c *Config) { c.Logging.Format = "xml" }, wantErr: "logging.format"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
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

// TestToRouterConfig checks every loaded field reaches the runtime config.
func TestToRouterConfig(t *testing.T) {
	cfg := Defaults()
	cfg.Router.ListenAddr = ":9443"
	cfg.Router.LabelSelector = "app=custom"
	cfg.Router.SNIAnnotationKey = "example.com/sni"
	cfg.Router.RefreshInterval = 30 * time.Second
	cfg.Router.DialTimeout = 2 * time.Second

	assert.Equal(t, remoteagentrouter.Config{
		ListenAddr:       ":9443",
		LabelSelector:    "app=custom",
		SNIAnnotationKey: "example.com/sni",
		RefreshInterval:  30 * time.Second,
		DialTimeout:      2 * time.Second,
	}, cfg.ToRouterConfig())
}
