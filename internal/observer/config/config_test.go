// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_WithDefaults(t *testing.T) {
	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.Equal(t, 9097, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.False(t, cfg.Auth.EnableAuth)
	assert.Equal(t, 10000, cfg.Logging.MaxLogLimit)
	assert.Equal(t, "http://logs-adapter:9098", cfg.Adapters.LogsAdapterURL)
	assert.Equal(t, "http://tracing-adapter:9100", cfg.Adapters.TracingAdapterURL)
	assert.Equal(t, "http://metrics-adapter:9099", cfg.Adapters.MetricsAdapterURL)
	assert.Equal(t, "http://finops-adapter:9101", cfg.Adapters.FinOpsAdapterURL)
	assert.Equal(t, 30*time.Second, cfg.Adapters.FinOpsAdapterTimeout)
}

func TestLoad_FinOpsAdapterEnvOverride(t *testing.T) {
	t.Setenv("FINOPS_ADAPTER_URL", "http://custom-finops:1234")
	t.Setenv("FINOPS_ADAPTER_TIMEOUT", "45s")

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.Equal(t, "http://custom-finops:1234", cfg.Adapters.FinOpsAdapterURL)
	assert.Equal(t, 45*time.Second, cfg.Adapters.FinOpsAdapterTimeout)
}

func TestLoad_WithEnvironmentVariables(t *testing.T) {
	t.Setenv("SERVER_PORT", "8080")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("AUTH_ENABLE_AUTH", "true")
	t.Setenv("LOGGING_MAX_LOG_LIMIT", "5000")

	cfg, err := Load()
	require.NoError(t, err, "Failed to load config")

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.True(t, cfg.Auth.EnableAuth)
	assert.Equal(t, 5000, cfg.Logging.MaxLogLimit)
}

func TestLoad_CORSAllowedOrigins(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "simple comma-separated origins",
			envValue: "http://localhost:3000,http://example.com",
			expected: []string{"http://localhost:3000", "http://example.com"},
		},
		{
			name:     "whitespace is trimmed",
			envValue: " http://a.com , http://b.com ",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "trailing comma produces no empty items",
			envValue: "http://a.com,http://b.com,",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "multiple trailing commas",
			envValue: "http://a.com,,http://b.com,,",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "whitespace-only entries are filtered",
			envValue: "http://a.com, , ,http://b.com",
			expected: []string{"http://a.com", "http://b.com"},
		},
		{
			name:     "single origin",
			envValue: "http://localhost:3000",
			expected: []string{"http://localhost:3000"},
		},
		{
			name:     "unset env var leaves empty slice",
			envValue: "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv("CORS_ALLOWED_ORIGINS", tt.envValue)
			} else {
				os.Unsetenv("CORS_ALLOWED_ORIGINS")
			}

			cfg, err := Load()
			require.NoError(t, err, "Failed to load config")

			require.Len(t, cfg.CORS.AllowedOrigins, len(tt.expected))
			for i, want := range tt.expected {
				assert.Equal(t, want, cfg.CORS.AllowedOrigins[i], "Origin[%d]", i)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	baseValidConfig := func() Config {
		return Config{
			Server: ServerConfig{
				Port:         8080,
				InternalPort: 8081,
			},
			Logging: LoggingConfig{
				MaxLogLimit: 1000,
			},
			Authz: AuthzConfig{
				ServiceURL: "http://localhost:8081",
				Timeout:    30 * time.Second,
			},
			UIDResolver: UIDResolverConfig{
				OpenChoreoAPIURL:  "http://localhost:9099",
				OAuthTokenURL:     "http://localhost:8080/oauth2/token",
				OAuthClientID:     "test-client",
				OAuthClientSecret: "test-secret",
				Timeout:           30 * time.Second,
			},
			Adapters: AdaptersConfig{
				MetricsAdapterURL:     "http://localhost:9090",
				MetricsAdapterTimeout: 30 * time.Second,
				FinOpsAdapterURL:      "http://localhost:9101",
				FinOpsAdapterTimeout:  30 * time.Second,
			},
		}
	}

	tests := []struct {
		name      string
		mutate    func(c *Config)
		expectErr bool
	}{
		{
			name:      "valid config",
			mutate:    func(c *Config) {},
			expectErr: false,
		},
		{
			name:      "invalid port - too low",
			mutate:    func(c *Config) { c.Server.Port = 0 },
			expectErr: true,
		},
		{
			name:      "invalid port - too high",
			mutate:    func(c *Config) { c.Server.Port = 99999 },
			expectErr: true,
		},
		{
			name:      "invalid max log limit",
			mutate:    func(c *Config) { c.Logging.MaxLogLimit = 0 },
			expectErr: true,
		},
		{
			name:      "missing authz service URL",
			mutate:    func(c *Config) { c.Authz.ServiceURL = "" },
			expectErr: true,
		},
		{
			name:      "invalid authz timeout",
			mutate:    func(c *Config) { c.Authz.Timeout = 0 },
			expectErr: true,
		},
		{
			name:      "missing metrics adapter URL",
			mutate:    func(c *Config) { c.Adapters.MetricsAdapterURL = "" },
			expectErr: true,
		},
		{
			name:      "invalid metrics adapter timeout",
			mutate:    func(c *Config) { c.Adapters.MetricsAdapterTimeout = 0 },
			expectErr: true,
		},
		{
			name:      "missing finops adapter URL",
			mutate:    func(c *Config) { c.Adapters.FinOpsAdapterURL = "" },
			expectErr: true,
		},
		{
			name:      "invalid finops adapter timeout",
			mutate:    func(c *Config) { c.Adapters.FinOpsAdapterTimeout = 0 },
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseValidConfig()
			tt.mutate(&cfg)
			err := cfg.validate()
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// uidResolutionResolver is the default UID resolution mode; passthrough is the
// seeded-data shortcut.
const uidResolutionResolver = "resolver"

// TestInsightsDefaultsLeaveTheFeatureOff pins that a chart or install which does
// not mention Delivery Insights gets it disabled. Aggregation in particular must
// default off: the aggregator has no leader election, so the chart refuses
// observer.replicas > 1 while it is enabled, and defaulting it on would fail the
// render of an existing scaled deployment that never opted in.
func TestInsightsDefaultsLeaveTheFeatureOff(t *testing.T) {
	// Load() reads the process environment, and anyone working on this feature is
	// likely to have INSIGHTS_* set in their shell -- which would make this assert
	// their environment rather than the defaults. Load() skips empty values, so
	// setting each to "" neutralizes it; t.Setenv restores the originals.
	for _, key := range []string{
		"INSIGHTS_STORE_BACKEND",
		"INSIGHTS_STORE_DSN",
		"INSIGHTS_UID_RESOLUTION",
		"INSIGHTS_AGGREGATION_ENABLED",
		"INSIGHTS_AGGREGATION_INTERVAL",
		"INSIGHTS_AGGREGATION_OVERLAP",
		"INSIGHTS_EVENTS_SOURCE_ENABLED",
		"INSIGHTS_ATTRIBUTION_WINDOW",
		"INSIGHTS_INCIDENT_LOOKBACK",
	} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	require.NoError(t, err)

	assert.False(t, cfg.Insights.AggregationEnabled,
		"aggregation must default off; enabling it caps the observer at one replica")
	assert.False(t, cfg.Insights.EventsSourceEnabled,
		"the events source must default off until a log module implements the sweep")
	assert.Equal(t, uidResolutionResolver, cfg.Insights.UIDResolution,
		"passthrough is a seeded-data shortcut and must not be the default")
}

func TestValidateInsightsStore(t *testing.T) {
	// newConfig returns a config whose only interesting fields are the store ones,
	// with aggregation off so validation stops after the store checks.
	newConfig := func(alertBackend, alertDSN, insightsBackend, insightsDSN string) *Config {
		c := &Config{}
		c.Alerting.AlertStoreBackend = alertBackend
		c.Alerting.AlertStoreDSN = alertDSN
		c.Insights.StoreBackend = insightsBackend
		c.Insights.StoreDSN = insightsDSN
		c.Insights.UIDResolution = uidResolutionResolver
		return c
	}

	t.Run("backend and DSN are inherited from the alert store when unset", func(t *testing.T) {
		c := newConfig("postgresql", "postgres://alerts", "", "")
		require.NoError(t, c.validateInsightsStore())
		assert.Equal(t, "postgresql", c.Insights.StoreBackend)
		assert.Equal(t, "postgres://alerts", c.Insights.StoreDSN,
			"sharing the alert store is the default, so its DSN carries over")
	})

	t.Run("a differing backend with no DSN is rejected", func(t *testing.T) {
		c := newConfig("sqlite", "file:/data/alerts.db", "postgresql", "")
		err := c.validateInsightsStore()
		require.Error(t, err, "inheriting a SQLite DSN into a PostgreSQL store would be nonsense")
		assert.Contains(t, err.Error(), "insights.store.dsn is required")
	})

	t.Run("an unknown backend is rejected", func(t *testing.T) {
		c := newConfig("sqlite", "file:/data/alerts.db", "mysql", "mysql://x")
		err := c.validateInsightsStore()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be 'sqlite' or 'postgresql'")
	})

	t.Run("backend is normalised", func(t *testing.T) {
		c := newConfig("sqlite", "file:/data/alerts.db", "  SQLite  ", "file:/data/x.db")
		require.NoError(t, c.validateInsightsStore())
		assert.Equal(t, "sqlite", c.Insights.StoreBackend)
	})

	t.Run("a SQLite DSN gains a busy timeout", func(t *testing.T) {
		c := newConfig("sqlite", "file:/data/alerts.db", "sqlite", "file:/data/insights.db")
		require.NoError(t, c.validateInsightsStore())
		assert.Contains(t, c.Insights.StoreDSN, "busy_timeout",
			"both stores may share one file, so a writer must wait rather than fail with SQLITE_BUSY")
	})

	t.Run("an existing busy timeout is preserved", func(t *testing.T) {
		dsn := "file:/data/insights.db?_pragma=busy_timeout(120000)"
		c := newConfig("sqlite", "file:/data/alerts.db", "sqlite", dsn)
		require.NoError(t, c.validateInsightsStore())
		assert.Equal(t, dsn, c.Insights.StoreDSN, "an operator's own timeout must not be overridden")
	})

	t.Run("a PostgreSQL DSN is left alone", func(t *testing.T) {
		c := newConfig("postgresql", "postgres://alerts", "postgresql", "postgres://insights")
		require.NoError(t, c.validateInsightsStore())
		assert.Equal(t, "postgres://insights", c.Insights.StoreDSN)
	})
}

func TestEnsureSQLiteBusyTimeout(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{"appends with ? when the DSN has no query", "file:/data/x.db",
			"file:/data/x.db?_pragma=busy_timeout(5000)"},
		{"appends with & when the DSN already has a query", "file:/data/x.db?_journal=WAL",
			"file:/data/x.db?_journal=WAL&_pragma=busy_timeout(5000)"},
		{"leaves an existing timeout untouched", "file:/data/x.db?_pragma=busy_timeout(1)",
			"file:/data/x.db?_pragma=busy_timeout(1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ensureSQLiteBusyTimeout(tt.dsn))
		})
	}
}

func TestValidateInsightsAggregation(t *testing.T) {
	newConfig := func(interval, overlap time.Duration) *Config {
		c := &Config{}
		c.Alerting.AlertStoreBackend = "sqlite"
		c.Alerting.AlertStoreDSN = "file:/data/alerts.db"
		c.Insights.UIDResolution = uidResolutionResolver
		c.Insights.AggregationEnabled = true
		c.Insights.AggregationInterval = interval
		c.Insights.AggregationOverlap = overlap
		// Valid values for the bounds this case is not exercising, so a failure
		// names the field under test rather than the first unset one.
		c.Insights.AttributionWindow = 24 * time.Hour
		c.Insights.IncidentLookback = 30 * 24 * time.Hour
		return c
	}

	t.Run("aggregation bounds are not checked while it is disabled", func(t *testing.T) {
		c := newConfig(0, -time.Second)
		c.Insights.AggregationEnabled = false
		require.NoError(t, c.validateInsights(),
			"an install that never enables aggregation must not have to supply its timings")
	})

	t.Run("a non-positive interval is rejected", func(t *testing.T) {
		err := newConfig(0, time.Minute).validateInsights()
		require.Error(t, err, "a zero tick would spin or never fire")
		assert.Contains(t, err.Error(), "insights.aggregation.interval must be positive")
	})

	t.Run("a negative overlap is rejected", func(t *testing.T) {
		err := newConfig(5*time.Minute, -time.Second).validateInsights()
		require.Error(t, err, "a negative overlap would move the window backwards")
		assert.Contains(t, err.Error(), "insights.aggregation.overlap must be non-negative")
	})

	t.Run("a zero overlap is allowed", func(t *testing.T) {
		require.NoError(t, newConfig(5*time.Minute, 0).validateInsights())
	})

	t.Run("a non-positive attribution window is rejected", func(t *testing.T) {
		c := newConfig(5*time.Minute, time.Minute)
		c.Insights.AttributionWindow = 0
		err := c.validateInsights()
		require.Error(t, err, "a zero window would attribute no incident to any deployment")
		assert.Contains(t, err.Error(), "insights.aggregation.attribution.window must be positive")
	})

	t.Run("a non-positive incident lookback is rejected", func(t *testing.T) {
		c := newConfig(5*time.Minute, time.Minute)
		c.Insights.IncidentLookback = 0
		err := c.validateInsights()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "insights.aggregation.incident.lookback must be positive")
	})

	t.Run("passthrough stays accepted", func(t *testing.T) {
		c := newConfig(5*time.Minute, time.Minute)
		c.Insights.UIDResolution = "passthrough"
		require.NoError(t, c.validateInsights(),
			"passthrough serves seeded data without a control plane and must remain valid")
		assert.Equal(t, "passthrough", c.Insights.UIDResolution)
	})

	t.Run("an empty uid resolution mode normalises to resolver", func(t *testing.T) {
		c := newConfig(5*time.Minute, time.Minute)
		c.Insights.UIDResolution = "  "
		require.NoError(t, c.validateInsights())
		assert.Equal(t, uidResolutionResolver, c.Insights.UIDResolution)
	})

	t.Run("an unknown uid resolution mode is rejected", func(t *testing.T) {
		c := newConfig(5*time.Minute, time.Minute)
		c.Insights.UIDResolution = "guess"
		err := c.validateInsights()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must be 'resolver' or 'passthrough'")
	})
}
