// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

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
		// A secret-bearing capability outliving a dial-only one inverts the tighter
		// freeze it exists for, and lengthens the agent's read grant with it.
		{"secret ttl over capability ttl", func(c *RemoteConnectConfig) {
			c.TTLSeconds, c.SecretTTLSeconds = 600, 1800
		}, "secret_ttl_seconds"},
		{"negative agent max sessions", func(c *RemoteConnectConfig) { c.AgentMaxSessions = -1 }, "agent_max_sessions"},
		{"negative agent authorize rate", func(c *RemoteConnectConfig) { c.AgentAuthorizeRate = -1 }, "agent_authorize_rate"},
		// NaN slips past an ordered comparison, so it needs its own check.
		{"nan agent authorize rate", func(c *RemoteConnectConfig) {
			c.AgentAuthorizeRate = math.NaN()
		}, "agent_authorize_rate"},
		{"infinite agent authorize rate", func(c *RemoteConnectConfig) {
			c.AgentAuthorizeRate = math.Inf(1)
		}, "agent_authorize_rate"},
		{"negative agent authorize burst", func(c *RemoteConnectConfig) { c.AgentAuthorizeBurst = -1 }, "agent_authorize_burst"},
		// The agent floors a zero burst to one, so the pairing is rejected here.
		{"agent authorize rate without burst", func(c *RemoteConnectConfig) {
			c.AgentAuthorizeRate, c.AgentAuthorizeBurst = 20, 0
		}, "agent_authorize_burst"},
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
	// Defaults must bound an agent, not leave the limits opt-in.
	if d.AgentMaxSessions <= 0 {
		t.Errorf("default agent max sessions = %d, must bound a shared agent", d.AgentMaxSessions)
	}
	if d.AgentAuthorizeRate <= 0 || d.AgentAuthorizeBurst < 1 {
		t.Errorf("default agent authorize limit = %v/s burst %d, must pace the control plane",
			d.AgentAuthorizeRate, d.AgentAuthorizeBurst)
	}
}

// Zero means unlimited for both limits, and must clear the bounds.
func TestRemoteConnectValidateAcceptsUnlimitedAgentLimits(t *testing.T) {
	c := validRemoteConnect()
	c.AgentMaxSessions, c.AgentAuthorizeRate, c.AgentAuthorizeBurst = 0, 0, 0
	if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs.Error())
	}
}

// Zero means "fall back to ttl_seconds", and an equal value is the tightest freeze an
// operator can ask for without disabling the distinction.
func TestRemoteConnectValidateAcceptsSecretTTLAtOrBelowCapabilityTTL(t *testing.T) {
	for name, secret := range map[string]int{"zero": 0, "equal": 1800, "below": 600} {
		t.Run(name, func(t *testing.T) {
			c := validRemoteConnect()
			c.TTLSeconds, c.SecretTTLSeconds = 1800, secret
			if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) != 0 {
				t.Fatalf("secret_ttl_seconds=%d should be accepted, got %v", secret, errs.Error())
			}
		})
	}
}

// A renewal has to land before the capability expires and before the reaper's idle TTL,
// which is why the control plane picks the cadence.
func TestRenewAfterBeatsBothDeadlines(t *testing.T) {
	tests := []struct {
		name        string
		ttl         int
		secretTTL   int
		reaperTTL   int
		withSecrets bool
		want        time.Duration
	}{
		{
			name: "defaults leave a third of the lifetime for retries",
			ttl:  1800, secretTTL: 600, reaperTTL: 1800,
			want: 1200 * time.Second,
		},
		{
			name: "a secret-bearing capability renews against its shorter lifetime",
			ttl:  1800, secretTTL: 600, reaperTTL: 1800,
			withSecrets: true,
			want:        400 * time.Second,
		},
		{
			name: "a reaper TTL shorter than the capability TTL governs instead",
			ttl:  3600, secretTTL: 600, reaperTTL: 600,
			// 2/3 of 3600 is 2400, which would let the agent be reaped 1800s earlier.
			want: 540 * time.Second,
		},
		{
			name: "a tiny TTL still renews inside its own lifetime",
			ttl:  6, secretTTL: 0, reaperTTL: 1800,
			// The floor would push this to 10s, past the expiry it exists to beat.
			want: 4 * time.Second,
		},
		{
			name: "a reaper TTL below the fixed margin still governs",
			ttl:  1800, secretTTL: 600, reaperTTL: 45,
			// 45 cannot absorb the 60s margin, so the proportional deadline applies.
			want: 30 * time.Second,
		},
		{
			name: "a reaper TTL just above the fixed margin keeps a usable deadline",
			ttl:  1800, secretTTL: 600, reaperTTL: 75,
			// 75-60=15 is less than 2/3, so the proportional deadline is used.
			want: 50 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validRemoteConnect()
			c.TTLSeconds, c.SecretTTLSeconds, c.ReaperTTLSeconds = tt.ttl, tt.secretTTL, tt.reaperTTL
			got := c.RenewAfter(tt.withSecrets)
			if got != tt.want {
				t.Errorf("RenewAfter(%v) = %s, want %s", tt.withSecrets, got, tt.want)
			}
			// Whatever else it does, it must never schedule a renewal at or after the
			// expiry it is meant to precede.
			if ttl := c.CapabilityTTL(tt.withSecrets); got >= ttl {
				t.Errorf("RenewAfter(%v) = %s, which is not before the %s capability TTL",
					tt.withSecrets, got, ttl)
			}
		})
	}
}

// A bound shorter than one capability lifetime would refuse every session's first
// renewal, so it is rejected at startup.
func TestRemoteConnectValidateRejectsSessionBoundBelowCapabilityTTL(t *testing.T) {
	c := validRemoteConnect()
	c.TTLSeconds = 1800
	c.MaxSessionSeconds = 900
	errs := c.Validate(coreconfig.NewPath("remote_connect"))
	if len(errs) == 0 {
		t.Fatal("expected a session bound below the capability TTL to be rejected")
	}
	if !strings.Contains(errs.Error(), "max_session_seconds") {
		t.Errorf("error should name the offending field, got: %s", errs.Error())
	}
}

// Zero means unbounded: every renewal re-runs authorization, so the revocation window
// stays the renewal interval however long a session runs.
func TestRemoteConnectValidateAcceptsUnboundedSession(t *testing.T) {
	c := validRemoteConnect()
	c.MaxSessionSeconds = 0
	if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) != 0 {
		t.Errorf("unbounded session should be valid, got: %s", errs.Error())
	}
	if c.MaxSession() != 0 {
		t.Errorf("MaxSession() = %s, want 0", c.MaxSession())
	}
}

// The default must stay IfNotPresent: an installation that imports images into the node
// rather than pulling them would break under Always.
func TestRemoteConnectDefaultPullPolicyIsIfNotPresent(t *testing.T) {
	if got := RemoteConnectDefaults().AgentImagePullPolicy; got != string(corev1.PullIfNotPresent) {
		t.Errorf("default agent image pull policy = %q, want %q", got, corev1.PullIfNotPresent)
	}
}

func TestRemoteConnectValidatesPullPolicy(t *testing.T) {
	tests := []struct {
		policy  string
		wantErr bool
	}{
		{policy: "Always"},
		{policy: "IfNotPresent"},
		{policy: "Never"},
		{policy: "", wantErr: true},
		{policy: "always", wantErr: true},
		{policy: "Sometimes", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			c := validRemoteConnect()
			c.AgentImagePullPolicy = tt.policy
			errs := c.Validate(coreconfig.NewPath("remote_connect"))
			if gotErr := len(errs) > 0; gotErr != tt.wantErr {
				t.Errorf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
		})
	}
}

// Past the ceiling the bound overflows to a negative duration, which the handler reads
// as unbounded rather than as the long bound the operator asked for.
func TestRemoteConnectMaxSessionSecondsCeiling(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		wantErr bool
	}{
		{name: "the exact ceiling is usable", seconds: 9_223_372_036},
		{name: "one past the ceiling overflows", seconds: 9_223_372_037, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validRemoteConnect()
			c.MaxSessionSeconds = tt.seconds
			errs := c.Validate(coreconfig.NewPath("remote_connect"))
			if gotErr := len(errs) > 0; gotErr != tt.wantErr {
				t.Fatalf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
			if !tt.wantErr && c.MaxSession() <= 0 {
				t.Errorf("MaxSession() = %s, want a positive bound", c.MaxSession())
			}
		})
	}
}

// A reaper TTL this short leaves no cadence that both beats the reaper and respects the
// renewal floor, so the agent would be reaped while a session still needs it.
func TestRemoteConnectRejectsUnservableReaperTTL(t *testing.T) {
	tests := []struct {
		seconds int
		wantErr bool
	}{
		{seconds: 10, wantErr: true},
		{seconds: 15, wantErr: true},
		{seconds: 16},
		{seconds: 1800},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.seconds), func(t *testing.T) {
			c := validRemoteConnect()
			c.ReaperTTLSeconds = tt.seconds
			errs := c.Validate(coreconfig.NewPath("remote_connect"))
			if gotErr := len(errs) > 0; gotErr != tt.wantErr {
				t.Fatalf("Validate() errors = %v, wantErr %v", errs, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			// An accepted TTL must leave a cadence that lands before the reaper does.
			if got := c.RenewAfter(false); got >= c.ReaperTTL() {
				t.Errorf("RenewAfter = %s, want less than the %s reaper TTL", got, c.ReaperTTL())
			}
		})
	}
}

// The image must come from the chart, which pins it to the release version; a default
// here could only be a floating tag that outlives the binary it was built beside.
func TestRemoteConnectRequiresAnExplicitAgentImage(t *testing.T) {
	if got := RemoteConnectDefaults().AgentImage; got != "" {
		t.Errorf("default agent image = %q, want none", got)
	}
	c := validRemoteConnect()
	c.AgentImage = ""
	if errs := c.Validate(coreconfig.NewPath("remote_connect")); len(errs) == 0 {
		t.Error("an enabled remote-connect without an agent image should fail validation")
	}
}
