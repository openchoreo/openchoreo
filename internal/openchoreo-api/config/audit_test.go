// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditConfig_ValidPoliciesRoundTrip(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  enabled: true
  defaults:
    publish: true
  policies:
    - match:
        categories: [authorization]
      set:
        publish: false
    - match:
        actions: [create_workflow_run]
        actor_types: [service_account]
      set:
        publish: false
`)

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want none", err)
	}

	ps, err := cfg.Audit.BuildPolicySet()
	if err != nil {
		t.Fatalf("BuildPolicySet() error = %v", err)
	}
	if ps == nil {
		t.Fatal("BuildPolicySet() returned nil PolicySet with no error")
	}
}

func TestAuditConfig_RejectsCategoryInSet(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        actions: [create_project]
      set:
        category: management
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a category key inside set")
	}
	if !strings.Contains(err.Error(), "not operator-configurable") {
		t.Errorf("Validate() error = %q, want it to mention 'not operator-configurable'", err.Error())
	}
}

func TestAuditConfig_RejectsSuppressionByEntitlement(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        entitlements: [admin]
      set:
        publish: false
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for entitlement-based suppression")
	}
	if !strings.Contains(err.Error(), "actors/entitlements") {
		t.Errorf("Validate() error = %q, want it to mention actors/entitlements", err.Error())
	}
}

// TestNewLoader_RejectsMistypedSelectorKey guards against a typo under
// audit.policies[].match silently widening a rule instead of erroring — an
// unrecognized key like actor_typos would otherwise leave the selector empty,
// matching everything instead of nothing.
func TestNewLoader_RejectsMistypedSelectorKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
audit:
  policies:
    - match:
        actor_typos: [service_account]
      set:
        publish: false
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := NewLoader(path, nil)
	if err == nil {
		t.Fatal("NewLoader() = nil error, want a rejection of the unrecognized actor_typos key")
	}
	if !strings.Contains(err.Error(), "actor_typos") {
		t.Errorf("NewLoader() error = %q, want it to name the unrecognized key actor_typos", err.Error())
	}
}

func TestAuditConfig_RejectsInvalidCategoryValue(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        categories: [bogus]
      set:
        publish: false
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized category value")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("Validate() error = %q, want it to mention the allowed values", err.Error())
	}
}

func TestAuditConfig_RejectsInvalidOriginValue(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        origins: [bogus]
      set:
        publish: false
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized origin value")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("Validate() error = %q, want it to mention the allowed values", err.Error())
	}
}

func TestAuditConfig_RejectsInvalidResultValue(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        results: [bogus]
      set:
        publish: false
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized result value")
	}
	if !strings.Contains(err.Error(), "must be one of") {
		t.Errorf("Validate() error = %q, want it to mention the allowed values", err.Error())
	}
}

func TestAuditConfig_RejectsNonBooleanPublish(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        actions: [create_project]
      set:
        publish: "yes"
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for a non-boolean publish value")
	}
	if !strings.Contains(err.Error(), "must be a boolean") {
		t.Errorf("Validate() error = %q, want it to mention 'must be a boolean'", err.Error())
	}
}

func TestAuditConfig_RejectsUnknownSettingKey(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  policies:
    - match:
        actions: [create_project]
      set:
        bogus_setting: true
`)

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error for an unrecognized set key")
	}
	if !strings.Contains(err.Error(), "bogus_setting") {
		t.Errorf("Validate() error = %q, want it to name the unrecognized key bogus_setting", err.Error())
	}
}

func loadAuditTestConfig(t *testing.T, yamlContent string) Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yamlContent), 0o600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loader, err := NewLoader(path, nil)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}

	var cfg Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	return cfg
}
