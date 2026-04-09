// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// We test validate() directly since Load() depends on env vars and file I/O.

func validConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8080,
			InternalPort: 8081,
		},
		LLM: LLMConfig{
			ModelName: "claude-sonnet-4-20250514",
			APIKey:    "sk-test-key",
		},
		Report: ReportConfig{
			Backend:     "sqlite",
			DatabaseURI: "file:test.db",
		},
		Agent: AgentConfig{
			MaxConcurrentAnalyses: 5,
			AnalysisTimeout:       300,
		},
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	require.NoError(t, cfg.validate())
}

func TestValidate_InvalidServerPort(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Server.Port = 0
	require.Error(t, cfg.validate())

	cfg.Server.Port = 70000
	require.Error(t, cfg.validate())
}

func TestValidate_MaxValidPort(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Server.Port = 65535
	cfg.Server.InternalPort = 65534
	require.NoError(t, cfg.validate())
}

func TestValidate_InvalidInternalPort(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Server.InternalPort = -1
	require.Error(t, cfg.validate())
}

func TestValidate_SamePortAndInternalPort(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Server.InternalPort = cfg.Server.Port
	require.Error(t, cfg.validate())
}

func TestValidate_MissingModelName(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.LLM.ModelName = ""
	require.Error(t, cfg.validate())
}

func TestValidate_MissingAPIKey(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.LLM.APIKey = ""
	require.Error(t, cfg.validate())
}

func TestValidate_PostgreSQLWithoutURI(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Report.Backend = backendPostgreSQL
	cfg.Report.DatabaseURI = ""
	require.Error(t, cfg.validate())
}

func TestValidate_PostgreSQLWithURI(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Report.Backend = backendPostgreSQL
	cfg.Report.DatabaseURI = "postgres://localhost/rca"
	require.NoError(t, cfg.validate())
}

func TestValidate_SQLiteDefaultsURI(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Report.Backend = backendSQLite
	cfg.Report.DatabaseURI = ""
	require.NoError(t, cfg.validate())
	assert.NotEmpty(t, cfg.Report.DatabaseURI)
}

func TestValidate_InvalidBackend(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Report.Backend = "mysql"
	require.Error(t, cfg.validate())
}

func TestValidate_InvalidMaxConcurrent(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agent.MaxConcurrentAnalyses = 0
	require.Error(t, cfg.validate())
}

func TestValidate_InvalidAnalysisTimeout(t *testing.T) {
	t.Parallel()
	cfg := validConfig()
	cfg.Agent.AnalysisTimeout = -1
	require.Error(t, cfg.validate())
}

func TestGetDefaults(t *testing.T) {
	t.Parallel()
	defaults := getDefaults()

	server := defaults["server"].(map[string]interface{})
	assert.Equal(t, 8080, server["port"])
	assert.Equal(t, 8081, server["internal.port"])

	report := defaults["report"].(map[string]interface{})
	assert.Equal(t, "sqlite", report["backend"])

	agent := defaults["agent"].(map[string]interface{})
	assert.Equal(t, 5, agent["max.concurrent.analyses"])

	assert.Equal(t, "info", defaults["loglevel"])
}
