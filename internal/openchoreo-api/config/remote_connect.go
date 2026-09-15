// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// maxSessionSecondsCeiling is the largest value MaxSession can express without
// overflowing a time.Duration.
const maxSessionSecondsCeiling = int64(math.MaxInt64) / int64(time.Second)

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
	// SecretTTLSeconds is the capability lifetime when the capability carries secret
	// grants. Authorization is decided once, at resolve, and frozen into the capability
	// — the per-stream authorize callback re-checks nothing — so this is the whole
	// revocation window for a credential read, and is worth setting shorter than the
	// dial TTL. Zero falls back to TTLSeconds.
	SecretTTLSeconds int `koanf:"secret_ttl_seconds"`
	// MaxSessionSeconds bounds the total life of one `occ remote` session across
	// renewals, measured from its first resolve. Hygiene for forgotten sessions rather
	// than an access control: every renewal re-runs authorization, and a refused client
	// can start a new session. Zero means unbounded.
	MaxSessionSeconds int `koanf:"max_session_seconds"`
	// SecretsEnabled allows resolve to sign secret grants at all. An operator kill
	// switch independent of policy: with it off, no capability authorizes reading a
	// value, whatever roles grant, and `occ remote` behaves as it did before secret
	// resolution existed.
	SecretsEnabled bool `koanf:"secrets_enabled"`

	// AgentImage is the container image used for the provisioned remote-agent Deployment.
	AgentImage string `koanf:"agent_image"`
	// AgentImagePullPolicy is the pull policy for that image. IfNotPresent suits a tag
	// that changes per release; a floating tag needs Always for a rebuilt agent to be
	// fetched rather than served from the node's cache.
	AgentImagePullPolicy string `koanf:"agent_image_pull_policy"`
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
		TTLSeconds:            1800,  // 30 minutes
		SecretTTLSeconds:      600,   // 10 minutes: a credential read gets a tighter freeze
		MaxSessionSeconds:     43200, // 12 hours: covers a working session, bounds a forgotten one
		SecretsEnabled:        true,
		AgentImagePullPolicy:  string(corev1.PullIfNotPresent),
		AgentListenPort:       8443,
		SNISuffix:             "remote-connect",
		ReaperIntervalSeconds: 300,  // 5 minutes
		ReaperTTLSeconds:      1800, // 30 minutes idle
	}
}

// CapabilityTTL returns the capability lifetime to use for a capability, tightened to
// SecretTTLSeconds when it carries secret grants.
func (c *RemoteConnectConfig) CapabilityTTL(withSecrets bool) time.Duration {
	if withSecrets && c.SecretTTLSeconds > 0 {
		return time.Duration(c.SecretTTLSeconds) * time.Second
	}
	return time.Duration(c.TTLSeconds) * time.Second
}

// renewMargin keeps a renewal ahead of the reaper's idle TTL, so a slow resolve still
// refreshes the agent before it is considered idle.
const renewMargin = 60 * time.Second

// minRenewAfter floors the renewal interval so a misconfigured TTL pair cannot ask every
// connected occ to resolve continuously.
const minRenewAfter = 10 * time.Second

// RenewAfter returns how long occ should wait before renewing a capability with this
// lifetime. The cadence has to beat two deadlines: the capability's own expiry, and
// ReaperTTL, since a resolve is what refreshes a remote-agent once a session's streams
// have gone quiet. Two thirds of the lifetime leaves the remaining third for retries.
func (c *RemoteConnectConfig) RenewAfter(withSecrets bool) time.Duration {
	ttl := c.CapabilityTTL(withSecrets)
	if ttl <= 0 {
		return 0
	}
	after := ttl * 2 / 3
	if reaper := c.reaperDeadline(); reaper > 0 && reaper < after {
		after = reaper
	}
	if after < minRenewAfter {
		after = minRenewAfter
	}
	// The floor must not push a renewal past the expiry it exists to beat.
	if after >= ttl {
		after = ttl * 2 / 3
	}
	return after
}

// reaperDeadline is the latest a renewal can land and still refresh the agent before the
// reaper treats it as idle. A TTL too short for the fixed margin uses a proportional one.
func (c *RemoteConnectConfig) reaperDeadline() time.Duration {
	ttl := c.ReaperTTL()
	if ttl <= 0 {
		return 0
	}
	if fixed := ttl - renewMargin; fixed >= ttl*2/3 {
		return fixed
	}
	return ttl * 2 / 3
}

// MaxSession returns MaxSessionSeconds as a Duration; zero means unbounded.
func (c *RemoteConnectConfig) MaxSession() time.Duration {
	return time.Duration(c.MaxSessionSeconds) * time.Second
}

// GrantTTL returns how long the agent's read Role may go unread before the reaper
// removes it. A session reads once at startup, so the capability lifetime bounds it.
func (c *RemoteConnectConfig) GrantTTL() time.Duration {
	return c.CapabilityTTL(true)
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
	if err := coreconfig.MustBeOneOf(path.Child("agent_image_pull_policy"), c.AgentImagePullPolicy,
		[]string{string(corev1.PullAlways), string(corev1.PullIfNotPresent), string(corev1.PullNever)}); err != nil {
		errs = append(errs, err)
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
	// Zero means "fall back to ttl_seconds"; negative would mint already-expired
	// secret-bearing capabilities.
	if c.SecretTTLSeconds < 0 {
		errs = append(errs, coreconfig.MustBeGreaterThan(path.Child("secret_ttl_seconds"), c.SecretTTLSeconds, -1))
	}
	// A secret-bearing capability is meant to live no longer than a dial-only one, and
	// it also bounds the agent's read grant. A larger value inverts both.
	if c.SecretTTLSeconds > 0 && c.TTLSeconds > 0 && c.SecretTTLSeconds > c.TTLSeconds {
		errs = append(errs, coreconfig.MustBeInRange(
			path.Child("secret_ttl_seconds"), c.SecretTTLSeconds, 1, c.TTLSeconds))
	}
	// A bound below the capability lifetime would refuse every session's first renewal.
	if c.MaxSessionSeconds < 0 {
		errs = append(errs, coreconfig.MustBeGreaterThan(path.Child("max_session_seconds"), c.MaxSessionSeconds, -1))
	}
	if c.MaxSessionSeconds > 0 && c.TTLSeconds > 0 && c.MaxSessionSeconds < c.TTLSeconds {
		errs = append(errs, coreconfig.MustBeInRange(
			path.Child("max_session_seconds"), c.MaxSessionSeconds, c.TTLSeconds, 1<<31-1))
	}
	// Past the ceiling MaxSession overflows to a negative duration, which reads as
	// unbounded rather than as the long bound that was asked for.
	if c.MaxSessionSeconds > 0 {
		if err := coreconfig.MustBeLessThanOrEqual(path.Child("max_session_seconds"),
			int64(c.MaxSessionSeconds), maxSessionSecondsCeiling); err != nil {
			errs = append(errs, err)
		}
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("reaper_interval_seconds"), c.ReaperIntervalSeconds, 0); err != nil {
		errs = append(errs, err)
	}
	if err := coreconfig.MustBeGreaterThan(path.Child("reaper_ttl_seconds"), c.ReaperTTLSeconds, 0); err != nil {
		errs = append(errs, err)
	}
	// Below this no cadence both refreshes the agent before the reaper and respects the
	// renewal floor, so the agent would be reaped mid-session.
	if c.ReaperTTLSeconds > 0 && c.reaperDeadline() <= minRenewAfter {
		errs = append(errs, coreconfig.MustBeGreaterThan(path.Child("reaper_ttl_seconds"),
			c.ReaperTTLSeconds, int(minRenewAfter*3/2/time.Second)))
	}
	if err := coreconfig.MustBeInRange(path.Child("agent_listen_port"), c.AgentListenPort, 1, 65535); err != nil {
		errs = append(errs, err)
	}
	return errs
}
