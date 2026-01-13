// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/pflag"
)

type testServerConfig struct {
	Port        int           `koanf:"port"`
	ReadTimeout time.Duration `koanf:"read_timeout"`
}

type testLoggingConfig struct {
	Level string `koanf:"level"`
}

type testConfig struct {
	Server  testServerConfig  `koanf:"server"`
	Logging testLoggingConfig `koanf:"logging"`
}

func testDefaults() testConfig {
	return testConfig{
		Server: testServerConfig{
			Port:        8080,
			ReadTimeout: 15 * time.Second,
		},
		Logging: testLoggingConfig{
			Level: "info",
		},
	}
}

func TestLoader_StructDefaults(t *testing.T) {
	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected read_timeout 15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected level info, got %s", cfg.Logging.Level)
	}
}

func TestLoader_ConfigFileOverridesDefaults(t *testing.T) {
	configPath := filepath.Join("testdata", "test_config.yaml")

	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), configPath); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Config file overrides
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090 from config file, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read_timeout 30s from config file, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected level debug from config file, got %s", cfg.Logging.Level)
	}
}

func TestLoader_EnvVarsOverrideConfigFile(t *testing.T) {
	configPath := filepath.Join("testdata", "test_config.yaml")

	// Set env vars (double underscore for nesting)
	os.Setenv("OC_TEST__SERVER__PORT", "7070")
	os.Setenv("OC_TEST__LOGGING__LEVEL", "warn")
	defer func() {
		os.Unsetenv("OC_TEST__SERVER__PORT")
		os.Unsetenv("OC_TEST__LOGGING__LEVEL")
	}()

	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), configPath); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Env vars override config file
	if cfg.Server.Port != 7070 {
		t.Errorf("expected port 7070 from env var, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("expected level warn from env var, got %s", cfg.Logging.Level)
	}
	// Config file value preserved when no env override
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read_timeout 30s from config file, got %v", cfg.Server.ReadTimeout)
	}
}

func TestLoader_EnvVarTransformation(t *testing.T) {
	// Test underscore preservation in field names
	os.Setenv("OC_TEST__SERVER__READ_TIMEOUT", "45s")
	defer os.Unsetenv("OC_TEST__SERVER__READ_TIMEOUT")

	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Server.ReadTimeout != 45*time.Second {
		t.Errorf("expected read_timeout 45s from env var, got %v", cfg.Server.ReadTimeout)
	}
}

func TestLoader_MissingConfigFileFails(t *testing.T) {
	loader := NewLoader("OC_TEST")
	err := loader.LoadWithDefaults(testDefaults(), "nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestLoader_NoConfigFileOK(t *testing.T) {
	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults should succeed without config file: %v", err)
	}
}

func TestLoader_Set(t *testing.T) {
	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	// Override with Set (for CLI flags)
	if err := loader.Set("server.port", 6060); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.Server.Port != 6060 {
		t.Errorf("expected port 6060 from Set, got %d", cfg.Server.Port)
	}
}

func TestLoader_Raw(t *testing.T) {
	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	raw := loader.Raw()
	if raw == nil {
		t.Fatal("Raw() returned nil")
	}

	// Raw() returns nested map
	server, ok := raw["server"].(map[string]any)
	if !ok {
		t.Fatalf("expected server key in config map, got: %v", raw)
	}
	if server["port"] != 8080 {
		t.Errorf("expected port 8080 in Raw(), got %v", server["port"])
	}
}

func TestLoader_FlagsOverrideEnvVars(t *testing.T) {
	configPath := filepath.Join("testdata", "test_config.yaml")

	// Set env var
	os.Setenv("OC_TEST__SERVER__PORT", "7070")
	defer os.Unsetenv("OC_TEST__SERVER__PORT")

	// Set up flags with user-friendly names
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 0, "server port")
	if err := flags.Parse([]string{"--port=5050"}); err != nil {
		t.Fatalf("flags.Parse failed: %v", err)
	}

	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), configPath); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}
	if err := loader.LoadFlags(flags, map[string]string{
		"port": "server.port",
	}); err != nil {
		t.Fatalf("LoadFlags failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Flag should override env var
	if cfg.Server.Port != 5050 {
		t.Errorf("expected port 5050 from flag, got %d", cfg.Server.Port)
	}
}

func TestLoader_FlagsNotSetDoNotOverride(t *testing.T) {
	// Set env var
	os.Setenv("OC_TEST__SERVER__PORT", "7070")
	defer os.Unsetenv("OC_TEST__SERVER__PORT")

	// Set up flags but don't set the port flag
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 0, "server port")
	if err := flags.Parse([]string{}); err != nil { // No flags set
		t.Fatalf("flags.Parse failed: %v", err)
	}

	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}
	if err := loader.LoadFlags(flags, map[string]string{
		"port": "server.port",
	}); err != nil {
		t.Fatalf("LoadFlags failed: %v", err)
	}

	var cfg testConfig
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Env var should be used since flag was not explicitly set
	if cfg.Server.Port != 7070 {
		t.Errorf("expected port 7070 from env var, got %d", cfg.Server.Port)
	}
}

// validatingConfig implements Validator
type validatingConfig struct {
	Server testServerConfig `koanf:"server"`
}

func (c *validatingConfig) Validate() error {
	if c.Server.Port <= 0 {
		return fmt.Errorf("server.port must be positive")
	}
	return nil
}

func TestLoader_UnmarshalAndValidate(t *testing.T) {
	loader := NewLoader("OC_TEST")
	if err := loader.LoadWithDefaults(testDefaults(), ""); err != nil {
		t.Fatalf("LoadWithDefaults failed: %v", err)
	}

	var cfg validatingConfig
	if err := loader.UnmarshalAndValidate("", &cfg); err != nil {
		t.Fatalf("UnmarshalAndValidate failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoader_UnmarshalAndValidate_Fails(t *testing.T) {
	loader := NewLoader("OC_TEST")
	// Load with invalid port
	if err := loader.Set("server.port", 0); err != nil {
		t.Fatalf("loader.Set failed: %v", err)
	}

	var cfg validatingConfig
	err := loader.UnmarshalAndValidate("", &cfg)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
