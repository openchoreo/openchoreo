// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"
	"gopkg.in/yaml.v3"

	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// MaxLimit is the maximum number of results that can be returned by any query endpoint.
// This matches the OpenAPI spec's maximum for the limit field.
const MaxLimit = 1000

// Supported SQL store backends for the alert/incident and delivery insights stores.
const (
	storeBackendSQLite     = "sqlite"
	storeBackendPostgreSQL = "postgresql"
)

// Config holds all configuration for the logging service
type Config struct {
	Server      ServerConfig      `koanf:"server"`
	Auth        AuthConfig        `koanf:"auth"`
	Authz       AuthzConfig       `koanf:"authz"`
	Logging     LoggingConfig     `koanf:"logging"`
	Alerting    AlertingConfig    `koanf:"alerting"`
	Insights    InsightsConfig    `koanf:"insights"`
	Adapters    AdaptersConfig    `koanf:"adapters"`
	UIDResolver UIDResolverConfig `koanf:"uid_resolver"`
	CORS        CORSConfig        `koanf:"cors"`
	LogLevel    string            `koanf:"loglevel"`
}

// AdaptersConfig holds adapter configuration
type AdaptersConfig struct {
	LogsAdapterURL     string        `koanf:"logs.adapter.url"`
	LogsAdapterTimeout time.Duration `koanf:"logs.adapter.timeout"`

	TracingAdapterURL     string        `koanf:"tracing.adapter.url"`
	TracingAdapterTimeout time.Duration `koanf:"tracing.adapter.timeout"`

	MetricsAdapterURL     string        `koanf:"metrics.adapter.url"`
	MetricsAdapterTimeout time.Duration `koanf:"metrics.adapter.timeout"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port            int           `koanf:"port"`
	InternalPort    int           `koanf:"internal.port"`
	ReadTimeout     time.Duration `koanf:"read.timeout"`
	WriteTimeout    time.Duration `koanf:"write.timeout"`
	ShutdownTimeout time.Duration `koanf:"shutdown.timeout"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins []string `koanf:"allowed.origins"`
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret    string                   `koanf:"jwt.secret"`
	EnableAuth   bool                     `koanf:"enable.auth"`
	RequiredRole string                   `koanf:"required.role"`
	SubjectTypes []subject.UserTypeConfig `koanf:"subject_types"`
}

// AuthzConfig holds authorization configuration
type AuthzConfig struct {
	ServiceURL            string        `koanf:"service.url"`
	Timeout               time.Duration `koanf:"timeout"`
	TLSInsecureSkipVerify bool          `koanf:"tls.insecure.skip.verify"`
	// Disabled skips all PDP authorization checks (mirrors JWT_DISABLED).
	// Development/demo affordance only — never enable in production.
	Disabled bool `koanf:"disabled"`
}

// LoggingConfig holds application logging configuration
type LoggingConfig struct {
	MaxLogLimit          int `koanf:"max.log.limit"`
	DefaultLogLimit      int `koanf:"default.log.limit"`
	DefaultBuildLogLimit int `koanf:"default.build.log.limit"`
	MaxLogLinesPerFile   int `koanf:"max.log.lines.per.file"`
}

// AlertingConfig holds configuration related to alerting features
type AlertingConfig struct {
	// RCAServiceURL is the base URL for the AI RCA (Root Cause Analysis) service.
	// Used for health checks and triggering RCA analysis.
	RCAServiceURL string `koanf:"rca.service.url"`
	// AIRCAEnabled controls whether AI-powered root cause analysis is enabled.
	AIRCAEnabled bool `koanf:"ai.rca.enabled"`
	// ObservabilityNamespace is the Kubernetes namespace where openchoreo-observability-plane is deployed.
	// Used for creating/listing PrometheusRule CRs for metric-based alerting.
	ObservabilityNamespace string `koanf:"observability.namespace"`
	// AlertStoreBackend controls where fired alert entries are persisted.
	// Supported values: sqlite (default), postgresql.
	AlertStoreBackend string `koanf:"alert.store.backend"`
	// AlertStoreDSN is the SQL connection string for alert entry storage.
	AlertStoreDSN string `koanf:"alert.store.dsn"`
	// AlertSuppressionWindow is the duration within which duplicate alerts
	// for the same alert rule are suppressed. Set to 0 to disable suppression.
	AlertSuppressionWindow time.Duration `koanf:"alert.suppression.window"`
	// FinOpsAgentURL is the base URL for the FinOps agent service.
	// Used for triggering AI cost analysis for budget alerts.
	FinOpsAgentURL string `koanf:"finops.agent.url"`
	// FinOpsAgentEnabled controls whether FinOps agent integration is enabled.
	FinOpsAgentEnabled bool `koanf:"finops.agent.enabled"`
}

// InsightsConfig holds configuration for the Delivery Insights (DORA metrics) store.
type InsightsConfig struct {
	// StoreBackend controls where delivery facts and metric rollups are persisted.
	// Supported values: sqlite, postgresql. Empty inherits the alert store backend.
	StoreBackend string `koanf:"store.backend"`
	// StoreDSN is the SQL connection string for delivery insights storage.
	// Empty inherits the alert store DSN so all observer stores share one database,
	// which keeps incident↔deployment attribution a local SQL join.
	StoreDSN string `koanf:"store.dsn"`
	// UIDResolution controls how insights search-scope names are translated to the
	// UIDs the store is keyed by. "resolver" (default) resolves via openchoreo-api;
	// "passthrough" treats names as UIDs directly — a development/demo affordance
	// for querying seeded dummy data without a control plane.
	UIDResolution string `koanf:"uid.resolution"`
	// AggregationEnabled runs the DORA aggregator in the observer process.
	AggregationEnabled bool `koanf:"aggregation.enabled"`
	// AggregationInterval is the aggregator tick interval.
	AggregationInterval time.Duration `koanf:"aggregation.interval"`
	// AggregationOverlap re-reads this much of the previous window each tick to
	// absorb source ingest lag.
	AggregationOverlap time.Duration `koanf:"aggregation.overlap"`
	// AttributionWindow caps how long after a deployment an incident may trigger
	// and still count against it (change failure attribution).
	AttributionWindow time.Duration `koanf:"aggregation.attribution.window"`
	// IncidentLookback is the rolling incident window rescanned every tick (late
	// incident resolutions do not bump ingestion timestamps, so pure watermark
	// increments would miss them).
	IncidentLookback time.Duration `koanf:"aggregation.incident.lookback"`
	// EventsSourceEnabled feeds the aggregator delivery lifecycle events read
	// from the logs adapter. Requires an adapter that supports the events query
	// reasons filter and searchAfter pagination; off by default until the
	// deployed adapter does.
	EventsSourceEnabled bool `koanf:"aggregation.events.source.enabled"`
}

// UIDResolverConfig holds configuration for the resource UID resolver
// which resolves resource names to UIDs via the openchoreo-api
type UIDResolverConfig struct {
	// OpenChoreoAPIURL is the base URL for the openchoreo-api service
	OpenChoreoAPIURL string `koanf:"openchoreo.api.url"`
	// OAuthTokenURL is the OAuth2 token endpoint URL for client credentials grant
	OAuthTokenURL string `koanf:"oauth.token.url"`
	// OAuthClientID is the OAuth2 client ID for authentication
	OAuthClientID string `koanf:"oauth.client.id"`
	// OAuthClientSecret is the OAuth2 client secret for authentication
	OAuthClientSecret string `koanf:"oauth.client.secret"`
	// OAuthScope is the optional OAuth2 scope to request in the token request
	OAuthScope string `koanf:"oauth.scope"`
	// TLSInsecureSkipVerify skips TLS certificate verification (for development)
	TLSInsecureSkipVerify bool `koanf:"tls.insecure.skip.verify"`
	// Timeout is the HTTP client timeout for API calls
	Timeout time.Duration `koanf:"timeout"`
	// MaxAuthRetry is the maximum number of additional attempts after a 401
	// response from openchoreo-api when resolving a resource UID.
	MaxAuthRetry int `koanf:"max.auth.retry"`
}

// Load loads configuration from environment variables and defaults
func Load() (*Config, error) {
	k := koanf.New(".")

	// Load defaults first
	if err := k.Load(confmap.Provider(getDefaults(), "."), nil); err != nil {
		return nil, fmt.Errorf("failed to load defaults: %w", err)
	}

	// Load auth config file for JWT subject resolution
	authConfigPath := os.Getenv("OBSERVER_AUTH_CONFIG_PATH")
	if authConfigPath == "" {
		authConfigPath = "auth-config.yaml"
	}

	var authCfg struct {
		Auth struct {
			SubjectTypes []subject.UserTypeConfig `yaml:"subject_types"`
		} `yaml:"auth"`
	}
	if _, err := os.Stat(authConfigPath); err == nil {
		data, err := os.ReadFile(authConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read auth config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &authCfg); err != nil {
			return nil, fmt.Errorf("failed to parse auth config file: %w", err)
		}
	}

	// Load environment variables for specific keys we care about
	envOverrides := make(map[string]interface{})

	// Define environment variable mappings
	envMappings := map[string]string{
		"SERVER_PORT":                           "server.port",
		"SERVER_INTERNAL_PORT":                  "server.internal.port",
		"SERVER_READ_TIMEOUT":                   "server.read.timeout",
		"SERVER_WRITE_TIMEOUT":                  "server.write.timeout",
		"SERVER_SHUTDOWN_TIMEOUT":               "server.shutdown.timeout",
		"AUTH_JWT_SECRET":                       "auth.jwt.secret",
		"AUTH_ENABLE_AUTH":                      "auth.enable.auth",
		"AUTH_REQUIRED_ROLE":                    "auth.required.role",
		"AUTHZ_SERVICE_URL":                     "authz.service.url",
		"AUTHZ_TIMEOUT":                         "authz.timeout",
		"AUTHZ_TLS_INSECURE_SKIP_VERIFY":        "authz.tls.insecure.skip.verify",
		"LOGGING_MAX_LOG_LIMIT":                 "logging.max.log.limit",
		"LOGGING_DEFAULT_LOG_LIMIT":             "logging.default.log.limit",
		"LOGGING_DEFAULT_BUILD_LOG_LIMIT":       "logging.default.build.log.limit",
		"LOGGING_MAX_LOG_LINES_PER_FILE":        "logging.max.log.lines.per.file",
		"RCA_SERVICE_URL":                       "alerting.rca.service.url",
		"AI_RCA_ENABLED":                        "alerting.ai.rca.enabled",
		"OBSERVABILITY_NAMESPACE":               "alerting.observability.namespace",
		"ALERT_STORE_BACKEND":                   "alerting.alert.store.backend",
		"ALERT_STORE_DSN":                       "alerting.alert.store.dsn",
		"ALERT_SUPPRESSION_WINDOW":              "alerting.alert.suppression.window",
		"FINOPS_AGENT_URL":                      "alerting.finops.agent.url",
		"FINOPS_AGENT_ENABLED":                  "alerting.finops.agent.enabled",
		"INSIGHTS_STORE_BACKEND":                "insights.store.backend",
		"INSIGHTS_STORE_DSN":                    "insights.store.dsn",
		"INSIGHTS_UID_RESOLUTION":               "insights.uid.resolution",
		"INSIGHTS_AGGREGATION_ENABLED":          "insights.aggregation.enabled",
		"INSIGHTS_AGGREGATION_INTERVAL":         "insights.aggregation.interval",
		"INSIGHTS_AGGREGATION_OVERLAP":          "insights.aggregation.overlap",
		"INSIGHTS_EVENTS_SOURCE_ENABLED":        "insights.aggregation.events.source.enabled",
		"INSIGHTS_ATTRIBUTION_WINDOW":           "insights.aggregation.attribution.window",
		"INSIGHTS_INCIDENT_LOOKBACK":            "insights.aggregation.incident.lookback",
		"AUTHZ_DISABLED":                        "authz.disabled",
		"LOG_LEVEL":                             "loglevel",
		"PORT":                                  "server.port",           // Common alias
		"INTERNAL_PORT":                         "server.internal.port",  // Common alias
		"JWT_SECRET":                            "auth.jwt.secret",       // Common alias
		"ENABLE_AUTH":                           "auth.enable.auth",      // Common alias
		"MAX_LOG_LIMIT":                         "logging.max.log.limit", // Common alias
		"LOGS_ADAPTER_URL":                      "adapters.logs.adapter.url",
		"LOGS_ADAPTER_TIMEOUT":                  "adapters.logs.adapter.timeout",
		"TRACING_ADAPTER_URL":                   "adapters.tracing.adapter.url",
		"TRACING_ADAPTER_TIMEOUT":               "adapters.tracing.adapter.timeout",
		"METRICS_ADAPTER_URL":                   "adapters.metrics.adapter.url",
		"METRICS_ADAPTER_TIMEOUT":               "adapters.metrics.adapter.timeout",
		"UID_RESOLVER_OPENCHOREO_API_URL":       "uid_resolver.openchoreo.api.url",
		"UID_RESOLVER_OAUTH_TOKEN_URL":          "uid_resolver.oauth.token.url",
		"UID_RESOLVER_OAUTH_CLIENT_ID":          "uid_resolver.oauth.client.id",
		"UID_RESOLVER_OAUTH_CLIENT_SECRET":      "uid_resolver.oauth.client.secret",
		"UID_RESOLVER_OAUTH_SCOPE":              "uid_resolver.oauth.scope",
		"UID_RESOLVER_TLS_INSECURE_SKIP_VERIFY": "uid_resolver.tls.insecure.skip.verify",
		"UID_RESOLVER_TIMEOUT":                  "uid_resolver.timeout",
		"UID_RESOLVER_MAX_AUTH_RETRY":           "uid_resolver.max.auth.retry",
	}

	// Check for environment variables and map them to nested structure
	for envKey, configKey := range envMappings {
		if value := os.Getenv(envKey); value != "" {
			var parsedValue interface{} = value

			// Split the config key and create nested structure
			parts := strings.Split(configKey, ".")
			if len(parts) == 1 {
				// Top-level key
				envOverrides[configKey] = parsedValue
			} else if len(parts) == 2 {
				// Nested key like "server.port"
				section := parts[0]
				key := parts[1]
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = parsedValue
			} else if len(parts) >= 3 {
				// Handle multi-part keys like "logging.max.log.limit"
				section := parts[0]
				key := strings.Join(parts[1:], ".")
				if envOverrides[section] == nil {
					envOverrides[section] = make(map[string]interface{})
				}
				envOverrides[section].(map[string]interface{})[key] = parsedValue
			}
		}
	}

	// Load environment overrides
	if len(envOverrides) > 0 {
		if err := k.Load(confmap.Provider(envOverrides, "."), nil); err != nil {
			return nil, fmt.Errorf("failed to load environment overrides: %w", err)
		}
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Parse CORS allowed origins from comma-separated env var
	if origins := os.Getenv("CORS_ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				cfg.CORS.AllowedOrigins = append(cfg.CORS.AllowedOrigins, o)
			}
		}
	}

	// Assign subject types from separately loaded auth config
	cfg.Auth.SubjectTypes = authCfg.Auth.SubjectTypes

	// Validate and sort subject types configuration
	if len(cfg.Auth.SubjectTypes) > 0 {
		if err := subject.ValidateConfig(cfg.Auth.SubjectTypes); err != nil {
			return nil, fmt.Errorf("invalid subject type config: %w", err)
		}
		subject.SortByPriority(cfg.Auth.SubjectTypes)
	}

	// Validate configuration
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &cfg, nil
}

// getDefaults returns the default configuration values
func getDefaults() map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"port":             9097,
			"internal.port":    8081,
			"read.timeout":     "30s",
			"write.timeout":    "30s",
			"shutdown.timeout": "10s",
		},
		"auth": map[string]interface{}{
			"enable.auth":   false,
			"jwt.secret":    "default-secret",
			"required.role": "user",
		},
		"authz": map[string]interface{}{
			"service.url":              "http://localhost:8080",
			"timeout":                  "30s",
			"tls.insecure.skip.verify": false,
		},
		"logging": map[string]interface{}{
			"max.log.limit":           10000,
			"default.log.limit":       100,
			"default.build.log.limit": 3000,
			"max.log.lines.per.file":  600000,
		},
		"alerting": map[string]interface{}{
			"rca.service.url":          "http://sre-agent:8080",
			"ai.rca.enabled":           false,
			"observability.namespace":  "openchoreo-observability-plane",
			"alert.store.backend":      "sqlite",
			"alert.store.dsn":          "file:/data/alerts.db?_journal=WAL",
			"alert.suppression.window": "1h",
			"finops.agent.url":         "http://finops-agent:8080",
			"finops.agent.enabled":     false,
		},
		"insights": map[string]interface{}{
			"store.backend":                     "",
			"store.dsn":                         "",
			"uid.resolution":                    "resolver",
			"aggregation.enabled":               true,
			"aggregation.interval":              "5m",
			"aggregation.overlap":               "10m",
			"aggregation.attribution.window":    "24h",
			"aggregation.incident.lookback":     "720h", // 30 days
			"aggregation.events.source.enabled": false,
		},
		"adapters": map[string]interface{}{
			"logs.adapter.url":        "http://logs-adapter:9098",
			"logs.adapter.timeout":    "30s",
			"tracing.adapter.url":     "http://tracing-adapter:9100",
			"tracing.adapter.timeout": "30s",
			"metrics.adapter.url":     "http://metrics-adapter:9099",
			"metrics.adapter.timeout": "30s",
		},
		"uid_resolver": map[string]interface{}{
			"openchoreo.api.url":       "http://api.openchoreo.localhost:9099",
			"oauth.token.url":          "http://thunder.openchoreo.localhost:8080/oauth2/token",
			"oauth.client.id":          "openchoreo-observer-resource-reader-client",
			"oauth.client.secret":      "openchoreo-observer-resource-reader-client-secret",
			"tls.insecure.skip.verify": false,
			"timeout":                  "30s",
			"max.auth.retry":           2,
		},
		"loglevel": "info",
	}
}

// validateInsights normalizes and validates the delivery insights section.
func (c *Config) validateInsights() error {
	c.Insights.UIDResolution = strings.ToLower(strings.TrimSpace(c.Insights.UIDResolution))
	switch c.Insights.UIDResolution {
	case "":
		c.Insights.UIDResolution = "resolver"
	case "resolver", "passthrough":
	default:
		return fmt.Errorf("insights.uid.resolution must be 'resolver' or 'passthrough'")
	}
	if !c.Insights.AggregationEnabled {
		return nil
	}
	if c.Insights.AggregationInterval <= 0 {
		return fmt.Errorf("insights.aggregation.interval must be positive")
	}
	if c.Insights.AggregationOverlap < 0 {
		return fmt.Errorf("insights.aggregation.overlap must be non-negative")
	}
	if c.Insights.AttributionWindow <= 0 {
		return fmt.Errorf("insights.aggregation.attribution.window must be positive")
	}
	if c.Insights.IncidentLookback <= 0 {
		return fmt.Errorf("insights.aggregation.incident.lookback must be positive")
	}
	return nil
}

func (c *Config) validate() error {
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	if c.Server.InternalPort <= 0 || c.Server.InternalPort > 65535 {
		return fmt.Errorf("invalid server internal port: %d", c.Server.InternalPort)
	}

	if c.Server.InternalPort == c.Server.Port {
		return fmt.Errorf("server internal port must differ from server port: %d", c.Server.Port)
	}

	if c.Logging.MaxLogLimit <= 0 {
		return fmt.Errorf("max log limit must be positive")
	}

	if c.Authz.ServiceURL == "" {
		return fmt.Errorf("authz service URL is required")
	}
	if c.Authz.Timeout <= 0 {
		return fmt.Errorf("authz timeout must be positive")
	}

	if c.UIDResolver.OpenChoreoAPIURL == "" {
		return fmt.Errorf("uid resolver openchoreo API URL is required")
	}
	if c.UIDResolver.OAuthTokenURL == "" {
		return fmt.Errorf("uid resolver oauth token URL is required")
	}
	if c.UIDResolver.OAuthClientID == "" {
		return fmt.Errorf("uid resolver oauth client ID is required")
	}
	if c.UIDResolver.OAuthClientSecret == "" {
		return fmt.Errorf("uid resolver oauth client secret is required")
	}
	if c.UIDResolver.Timeout <= 0 {
		return fmt.Errorf("uid resolver timeout must be positive")
	}
	if c.UIDResolver.MaxAuthRetry < 0 {
		return fmt.Errorf("uid resolver max.auth.retry must be non-negative")
	}

	c.Alerting.AlertStoreBackend = strings.ToLower(strings.TrimSpace(c.Alerting.AlertStoreBackend))
	switch c.Alerting.AlertStoreBackend {
	case "", storeBackendSQLite:
		c.Alerting.AlertStoreBackend = storeBackendSQLite
		if strings.TrimSpace(c.Alerting.AlertStoreDSN) == "" {
			c.Alerting.AlertStoreDSN = "file:/data/alerts.db?_journal=WAL"
		}
	case storeBackendPostgreSQL:
		if strings.TrimSpace(c.Alerting.AlertStoreDSN) == "" {
			return fmt.Errorf("alert.store.dsn is required when alert.store.backend=postgresql")
		}
	default:
		return fmt.Errorf("alert.store.backend must be 'sqlite' or 'postgresql'")
	}

	// The insights store defaults to sharing the alert store database so that
	// incident↔deployment attribution stays a local SQL join.
	c.Insights.StoreBackend = strings.ToLower(strings.TrimSpace(c.Insights.StoreBackend))
	if c.Insights.StoreBackend == "" {
		c.Insights.StoreBackend = c.Alerting.AlertStoreBackend
	}
	switch c.Insights.StoreBackend {
	case storeBackendSQLite, storeBackendPostgreSQL:
	default:
		return fmt.Errorf("insights.store.backend must be 'sqlite' or 'postgresql'")
	}
	if strings.TrimSpace(c.Insights.StoreDSN) == "" {
		if c.Insights.StoreBackend != c.Alerting.AlertStoreBackend {
			return fmt.Errorf(
				"insights.store.dsn is required when insights.store.backend differs from alert.store.backend")
		}
		c.Insights.StoreDSN = c.Alerting.AlertStoreDSN
	}
	if err := c.validateInsights(); err != nil {
		return err
	}

	// Validate and normalize MetricsAdapter configuration
	if c.Adapters.MetricsAdapterURL == "" {
		return fmt.Errorf("metrics adapter URL is required")
	}
	// Strip trailing slash to prevent double slashes in client paths
	c.Adapters.MetricsAdapterURL = strings.TrimRight(c.Adapters.MetricsAdapterURL, "/")

	if c.Adapters.MetricsAdapterTimeout <= 0 {
		return fmt.Errorf("metrics adapter timeout must be positive")
	}

	return nil
}
