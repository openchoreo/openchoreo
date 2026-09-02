// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/auditconfig"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/subject"
)

// writeAuthConfig writes body to a temp file and returns its path.
func writeAuthConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "auth-config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestLoadAuditConfig_NoFileKeepsDefaults covers the common deployment: no
// supplementary YAML at all. Audit must come up enabled and publishing — a
// zero AuditConfig would silently disable audit entirely.
func TestLoadAuditConfig_NoFileKeepsDefaults(t *testing.T) {
	var cfg auditconfig.AuditConfig
	require.NoError(t, loadAuditConfig(filepath.Join(t.TempDir(), "absent.yaml"), &cfg))

	assert.Equal(t, auditconfig.AuditDefaults(), cfg)
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.Defaults.Publish)
}

// TestLoadAuditConfig_NoAuditKeyKeepsDefaults covers a file that exists for
// auth.subject_types but says nothing about audit.
func TestLoadAuditConfig_NoAuditKeyKeepsDefaults(t *testing.T) {
	path := writeAuthConfig(t, "auth:\n  subject_types:\n    - type: user\n")

	var cfg auditconfig.AuditConfig
	require.NoError(t, loadAuditConfig(path, &cfg))

	assert.Equal(t, auditconfig.AuditDefaults(), cfg)
}

// TestLoadAuditConfig_DecodesPolicies is why this loads through koanf rather
// than the plain yaml.Unmarshal the auth section uses: yaml.v3 would mangle
// the underscore key actor_types to "actortypes" and silently drop it,
// producing a selector that matches nothing.
func TestLoadAuditConfig_DecodesPolicies(t *testing.T) {
	path := writeAuthConfig(t, `
audit:
  enabled: true
  defaults:
    publish: true
  policies:
    - match:
        actor_types: [user]
        results: [denied]
      set:
        publish: false
`)

	var cfg auditconfig.AuditConfig
	require.NoError(t, loadAuditConfig(path, &cfg))

	require.Len(t, cfg.Policies, 1)
	assert.Equal(t, []string{"user"}, cfg.Policies[0].Match.ActorTypes,
		"actor_types must decode by its koanf tag, not be dropped as an unmatched field")
	assert.Equal(t, []string{"denied"}, cfg.Policies[0].Match.Results)
	assert.Equal(t, false, cfg.Policies[0].Set["publish"])
}

// TestLoadAuditConfig_RejectsUnknownKey covers the ErrorUnused decoder
// setting. An empty match matches everything, so a typo'd selector must fail
// startup rather than silently widen a narrowing rule into a blanket one.
func TestLoadAuditConfig_RejectsUnknownKey(t *testing.T) {
	path := writeAuthConfig(t, `
audit:
  enabled: true
  policies:
    - match:
        actor_typos: [user]
      set:
        publish: false
`)

	var cfg auditconfig.AuditConfig
	err := loadAuditConfig(path, &cfg)
	require.Error(t, err, "an unrecognized selector key must be rejected, not silently ignored")
	assert.Contains(t, err.Error(), "actor_typos")
}

// TestKnownActorTypes covers the set actor_types selectors are validated
// against: the two values ExtractActor produces unconfigured ("anonymous",
// and "user" as its fallback) plus every configured type. Miss one and a
// valid entry is rejected at startup; include too much and a typo matches
// nothing.
func TestKnownActorTypes(t *testing.T) {
	t.Run("defaults with no subject types", func(t *testing.T) {
		c := &AuthConfig{}
		assert.Equal(t, []string{"anonymous", "user"}, c.KnownActorTypes())
	})

	t.Run("configured types are appended, sorted and de-duplicated", func(t *testing.T) {
		c := &AuthConfig{SubjectTypes: []subject.UserTypeConfig{
			{Type: "service_account"},
			{Type: "user"},  // already present, must not duplicate
			{Type: "agent"}, // must sort before service_account
			{Type: ""},      // must be skipped
		}}
		assert.Equal(t, []string{"anonymous", "user", "agent", "service_account"}, c.KnownActorTypes())
	})
}
