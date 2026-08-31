// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

// TestNewLoader_ShippedConfigEnablesAudit loads the shipped config.yaml and
// asserts audit ends up enabled — a struct tag typo or missing registration
// could silently turn it off without failing a test built on a literal
// config.AuditConfig{Enabled: true} instead of the real loader.
func TestNewLoader_ShippedConfigEnablesAudit(t *testing.T) {
	loader, err := NewLoader("../../../cmd/openchoreo-api/config.yaml", nil)
	if err != nil {
		t.Fatalf("NewLoader() error = %v", err)
	}

	var cfg Config
	if err := loader.Unmarshal("", &cfg); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !cfg.Audit.Enabled {
		t.Error("Audit.Enabled = false, want true for the shipped config.yaml")
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want the shipped config.yaml to pass validation", err)
	}
}
