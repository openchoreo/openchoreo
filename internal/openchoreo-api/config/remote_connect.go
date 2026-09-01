// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"time"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// RemoteConnectConfig configures the `occ remote` resolve/authorize endpoints, the signed
// capability they issue, and the per-project+env remote-agent the control plane
// provisions into the data plane. The capability is minted by
// openchoreo-api and verified by its own authorize endpoint (the remote-agent holds no
// key); the byte path runs occ -> remote-agent L4 Service directly. When disabled,
// none of these endpoints are served.
type RemoteConnectConfig struct {
	// Enabled controls whether the remote-connect resolve and authorize endpoints are
	// served (and remote-agents provisioned).
	Enabled bool `koanf:"enabled"`
	// SigningKeyPath is the path to the PEM-encoded Ed25519 private key used to sign
	// and verify capabilities.
	SigningKeyPath string `koanf:"signing_key_path"`
	// KeyID is set as the JWT `kid` header for key rotation.
	KeyID string `koanf:"key_id"`
	// Issuer is the capability JWT `iss` claim.
	Issuer string `koanf:"issuer"`
	// TTLSeconds is the capability lifetime in seconds.
	TTLSeconds int `koanf:"ttl_seconds"`

	// AgentImage is the container image used for the provisioned remote-agent Deployment.
	AgentImage string `koanf:"agent_image"`
	// AgentListenPort is the TLS tunnel port the remote-agent listens on (ClusterIP
	// Service targets it; the shared SNI router forwards to it).
	AgentListenPort int `koanf:"agent_listen_port"`
	// EntrypointAddress is the "host:port" of the shared remote-connect SNI router that
	// occ dials — a single per-data-plane L4 entrypoint that routes to each agent by
	// SNI. resolve returns this as the agent endpoint for every project+env.
	EntrypointAddress string `koanf:"entrypoint_address"`
	// SNISuffix is appended to the data-plane namespace to form each agent's SNI host
	// (e.g. "<dp-namespace>.remote-connect"). Defaults to "remote-connect".
	SNISuffix string `koanf:"sni_suffix"`
	// AuthorizeURL is the control-plane authorize endpoint the remote-agent calls per
	// stream, as reachable from the data plane. Injected into the agent Deployment.
	AuthorizeURL string `koanf:"authorize_url"`
	// AuthorizeInsecure tells the remote-agent to skip TLS verification when calling the
	// control plane (development only).
	AuthorizeInsecure bool `koanf:"authorize_insecure"`
	// ReaperIntervalSeconds is how often the idle-remote-agent reaper runs.
	ReaperIntervalSeconds int `koanf:"reaper_interval_seconds"`
	// ReaperTTLSeconds is how long a remote-agent may be idle (no resolve refreshing its
	// last-used annotation) before the reaper deletes it.
	ReaperTTLSeconds int `koanf:"reaper_ttl_seconds"`
}

// RemoteConnectDefaults returns the default remote-connect configuration.
func RemoteConnectDefaults() RemoteConnectConfig {
	return RemoteConnectConfig{
		Enabled:               false,
		Issuer:                "openchoreo-control-plane",
		KeyID:                 "remote-connect-1",
		TTLSeconds:            1800, // 30 minutes
		AgentImage:            "ghcr.io/openchoreo/remote-agent:latest",
		AgentListenPort:       8443,
		SNISuffix:             "remote-connect",
		ReaperIntervalSeconds: 300,  // 5 minutes
		ReaperTTLSeconds:      1800, // 30 minutes idle
	}
}

// ReaperInterval returns ReaperIntervalSeconds as a Duration.
func (c *RemoteConnectConfig) ReaperInterval() time.Duration {
	return time.Duration(c.ReaperIntervalSeconds) * time.Second
}

// ReaperTTL returns ReaperTTLSeconds as a Duration.
func (c *RemoteConnectConfig) ReaperTTL() time.Duration {
	return time.Duration(c.ReaperTTLSeconds) * time.Second
}

// Validate validates the remote-connect configuration.
func (c *RemoteConnectConfig) Validate(path *coreconfig.Path) coreconfig.ValidationErrors {
	var errs coreconfig.ValidationErrors
	if !c.Enabled {
		return errs
	}
	if c.SigningKeyPath == "" {
		errs = append(errs, coreconfig.Required(path.Child("signing_key_path")))
	}
	if c.AgentImage == "" {
		errs = append(errs, coreconfig.Required(path.Child("agent_image")))
	}
	if c.AuthorizeURL == "" {
		errs = append(errs, coreconfig.Required(path.Child("authorize_url")))
	}
	if c.EntrypointAddress == "" {
		errs = append(errs, coreconfig.Required(path.Child("entrypoint_address")))
	}
	// Non-positive durations panic time.NewTicker in the reaper loop, and a zero TTL
	// would mint already-expired capabilities. Reject at startup rather than crashing.
	if err := coreconfig.MustBeGreaterThan(path.Child("ttl_seconds"), c.TTLSeconds, 0); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("reaper_interval_seconds"), c.ReaperIntervalSeconds, 0); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("reaper_ttl_seconds"), c.ReaperTTLSeconds, 0); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeInRange(path.Child("agent_listen_port"), c.AgentListenPort, 1, 65535); err != nil {
		errs = append(errs, err)
	}
	return errs
}
