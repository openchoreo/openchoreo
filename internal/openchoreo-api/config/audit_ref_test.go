// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

func TestAuditConfig_LoadsObservabilityPlaneRef(t *testing.T) {
	cfg := loadAuditTestConfig(t, `
audit:
  enabled: false
  defaults:
    publish: true
  observability_plane_ref:
    kind: ObservabilityPlane
    name: audit
    namespace: platform
`)

	if cfg.Audit.Enabled {
		t.Error("Audit.Enabled = true, want false from the config file")
	}
	want := AuditObservabilityPlaneRef{Kind: "ObservabilityPlane", Name: "audit", Namespace: "platform"}
	if diff := cmp.Diff(want, cfg.Audit.ObservabilityPlaneRef); diff != "" {
		t.Errorf("ObservabilityPlaneRef mismatch (-want +got):\n%s", diff)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want none", err)
	}
}

func TestAuditObservabilityPlaneRef_Validate(t *testing.T) {
	tests := []struct {
		name         string
		ref          AuditObservabilityPlaneRef
		auditEnabled bool
		wantFields   []string
	}{
		{
			name: "defaults are valid",
			ref:  AuditDefaults().ObservabilityPlaneRef,
		},
		{
			name: "incomplete reference is ignored when audit is disabled",
			ref:  AuditObservabilityPlaneRef{Name: "default"},
		},
		{
			name:         "unset is rejected when audit is enabled",
			auditEnabled: true,
			wantFields:   []string{"audit.observability_plane_ref.kind", "audit.observability_plane_ref.name"},
		},
		{
			name:         "kind without name is rejected when audit is enabled",
			ref:          AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane"},
			auditEnabled: true,
			wantFields:   []string{"audit.observability_plane_ref.name"},
		},
		{
			name:         "name without kind is rejected when audit is enabled",
			ref:          AuditObservabilityPlaneRef{Name: "default"},
			auditEnabled: true,
			wantFields:   []string{"audit.observability_plane_ref.kind"},
		},
		{
			name:         "kind and name are enough when audit is enabled",
			ref:          AuditObservabilityPlaneRef{Kind: "ClusterObservabilityPlane", Name: "default"},
			auditEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.ref.validate(coreconfig.NewPath("audit").Child("observability_plane_ref"), tt.auditEnabled)

			gotFields := make([]string, 0, len(errs))
			for _, e := range errs {
				gotFields = append(gotFields, e.Field)
			}
			if diff := cmp.Diff(tt.wantFields, gotFields, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("error fields mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
