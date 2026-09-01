// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"strings"
	"testing"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// validRemoteConnect is a configuration that passes validation; each test perturbs one field.
func validRemoteConnect() RemoteConnectConfig {
	c := RemoteConnectDefaults()
	c.Enabled = true
	c.SigningKeyPath = "/etc/remote-connect/signing.pem"
	c.AgentImage = "ghcr.io/openchoreo/remote-agent:v1"
	c.AuthorizeURL = "https://api.example.com/api/v1/remote-connect:authorize"
	c.EntrypointAddress = "router.example.com:8443"
	return c
}

// TestRemoteConnectValidateRejectsUnusableNumbers: a non-positive reaper interval panics
// time.NewTicker inside the reaper goroutine, which would crash the API server rather
// than fail startup. The other bounds guard equally unusable values.
func TestRemoteConnectValidateRejectsUnusableNumbers(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*RemoteConnectConfig)
		wantField string
	}{
		{"zero reaper interval", func(c *RemoteConnectConfig) { c.ReaperIntervalSeconds = 0 }, "reaper_interval_seconds"},
		{"negative reaper interval", func(c *RemoteConnectConfig) { c.ReaperIntervalSeconds = -1 }, "reaper_interval_seconds"},
		{"zero reaper ttl", func(c *RemoteConnectConfig) { c.ReaperTTLSeconds = 0 }, "reaper_ttl_seconds"},
		{"zero capability ttl", func(c *RemoteConnectConfig) { c.TTLSeconds = 0 }, "ttl_seconds"},
		{"zero agent port", func(c *RemoteConnectConfig) { c.AgentListenPort = 0 }, "agent_listen_port"},
		{"agent port out of range", func(c *RemoteConnectConfig) { c.AgentListenPort = 70000 }, "agent_listen_port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validRemoteConnect()
			tt.mutate(&c)
			errs := c.Validate(coreconfig.NewPath("remote_connect"))
			if len(errs) == 0 {
				t.Fatalf("expected a validation error for %s", tt.wantField)
			}
			if !strings.Contains(errs.Error(), tt.wantField) {
				t.Errorf("error %q does not mention %s", errs.Error(), tt.wantField)
			}
		})
	}
}

func TestRemoteConnectValidateAcceptsValidConfig(t *testing.T) {
	c := validRemoteConnect()
	if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs.Error())
	}
}

// TestRemoteConnectValidateSkippedWhenDisabled: the numeric bounds must not block a
// disabled remote-connect, whose fields are never read.
func TestRemoteConnectValidateSkippedWhenDisabled(t *testing.T) {
	c := RemoteConnectConfig{Enabled: false}
	if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) != 0 {
		t.Fatalf("disabled remote-connect should not validate, got %v", errs.Error())
	}
}

// TestRemoteConnectDefaultsAreUsable guards the crash path directly: the shipped defaults
// must never produce a non-positive reaper interval.
func TestRemoteConnectDefaultsAreUsable(t *testing.T) {
	d := RemoteConnectDefaults()
	if d.Enabled {
		t.Error("remote-connect should default to disabled")
	}
	if d.ReaperInterval() <= 0 {
		t.Errorf("default reaper interval = %v, must be positive", d.ReaperInterval())
	}
	if d.ReaperTTL() <= 0 {
		t.Errorf("default reaper TTL = %v, must be positive", d.ReaperTTL())
	}
}
