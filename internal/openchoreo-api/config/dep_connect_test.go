// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// validDepConnect is a configuration that passes validation; each test perturbs one field.
func validDepConnect() DepConnectConfig {
	c := DepConnectDefaults()
	c.Enabled = true
	c.SigningKeyPath = "/etc/dep-connect/signing.pem"
	c.AgentImage = "ghcr.io/openchoreo/dep-agent:v1"
	c.AuthorizeURL = "https://api.example.com/api/v1/dep-connect:authorize"
	c.EntrypointAddress = "router.example.com:8443"
	return c
}

// TestDepConnectValidateRejectsUnusableNumbers: a non-positive reaper interval panics
// time.NewTicker inside the reaper goroutine, which would crash the API server rather
// than fail startup. The other bounds guard equally unusable values.
func TestDepConnectValidateRejectsUnusableNumbers(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*DepConnectConfig)
		wantField string
	}{
		{"zero reaper interval", func(c *DepConnectConfig) { c.ReaperIntervalSeconds = 0 }, "reaper_interval_seconds"},
		{"negative reaper interval", func(c *DepConnectConfig) { c.ReaperIntervalSeconds = -1 }, "reaper_interval_seconds"},
		{"zero reaper ttl", func(c *DepConnectConfig) { c.ReaperTTLSeconds = 0 }, "reaper_ttl_seconds"},
		{"zero capability ttl", func(c *DepConnectConfig) { c.TTLSeconds = 0 }, "ttl_seconds"},
		{"zero agent port", func(c *DepConnectConfig) { c.AgentListenPort = 0 }, "agent_listen_port"},
		{"agent port out of range", func(c *DepConnectConfig) { c.AgentListenPort = 70000 }, "agent_listen_port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validDepConnect()
			tt.mutate(&c)
			errs := c.Validate(coreconfig.NewPath("dep_connect"))
			if len(errs) == 0 {
				t.Fatalf("expected a validation error for %s", tt.wantField)
			}
			if !strings.Contains(errs.Error(), tt.wantField) {
				t.Errorf("error %q does not mention %s", errs.Error(), tt.wantField)
			}
		})
	}
}

func TestDepConnectValidateAcceptsValidConfig(t *testing.T) {
	c := validDepConnect()
	if errs := c.Validate(coreconfig.NewPath("dep_connect")); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs.Error())
	}
}

// TestDepConnectValidateSkippedWhenDisabled: the numeric bounds must not block a
// disabled dep-connect, whose fields are never read.
func TestDepConnectValidateSkippedWhenDisabled(t *testing.T) {
	c := DepConnectConfig{Enabled: false}
	if errs := c.Validate(coreconfig.NewPath("dep_connect")); len(errs) != 0 {
		t.Fatalf("disabled dep-connect should not validate, got %v", errs.Error())
	}
}

// TestDepConnectDefaultsAreUsable guards the crash path directly: the shipped defaults
// must never produce a non-positive reaper interval.
func TestDepConnectDefaultsAreUsable(t *testing.T) {
	d := DepConnectDefaults()
	if d.Enabled {
		t.Error("dep-connect should default to disabled")
	}
	if d.ReaperInterval() <= 0 {
		t.Errorf("default reaper interval = %v, must be positive", d.ReaperInterval())
	}
	if d.ReaperTTL() <= 0 {
		t.Errorf("default reaper TTL = %v, must be positive", d.ReaperTTL())
	}
}
