// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/auditconfig"
	coreconfig "github.com/openchoreo/openchoreo/internal/config"
)

// AuditConfig defines audit logging settings: the settings shared with observer
// plus the observability plane where audit logs are stored.
type AuditConfig struct {
	// squash is read by mapstructure when decoding; flatten by fatih/structs
	// when the loader converts Defaults() into a map.
	auditconfig.AuditConfig `koanf:",squash,flatten"`
	// Required when Enabled is true.
	ObservabilityPlaneRef AuditObservabilityPlaneRef `koanf:"observability_plane_ref"`
}

// AuditObservabilityPlaneRef references an ObservabilityPlane or a ClusterObservabilityPlane.
type AuditObservabilityPlaneRef struct {
	Kind      string `koanf:"kind"`
	Name      string `koanf:"name"`
	Namespace string `koanf:"namespace"`
}

type (
	// PolicyDefaultsConfig is auditconfig.PolicyDefaultsConfig.
	PolicyDefaultsConfig = auditconfig.PolicyDefaultsConfig
	// PolicyRuleConfig is auditconfig.PolicyRuleConfig.
	PolicyRuleConfig = auditconfig.PolicyRuleConfig
	// SelectorConfig is auditconfig.SelectorConfig.
	SelectorConfig = auditconfig.SelectorConfig
)

// AuditDefaults returns the default audit configuration.
func AuditDefaults() AuditConfig {
	return AuditConfig{AuditConfig: auditconfig.AuditDefaults()}
}

// Validate validates the audit configuration.
func (c *AuditConfig) Validate(
	path *coreconfig.Path, vocab auditconfig.Vocabulary, knownActorTypes []string,
) coreconfig.ValidationErrors {
	errs := c.AuditConfig.Validate(path, vocab, knownActorTypes)
	return append(errs, c.ObservabilityPlaneRef.validate(path.Child("observability_plane_ref"), c.Enabled)...)
}

func (r AuditObservabilityPlaneRef) validate(path *coreconfig.Path, auditEnabled bool) coreconfig.ValidationErrors {
	if !auditEnabled {
		return nil
	}

	var errs coreconfig.ValidationErrors
	if r.Kind == "" {
		errs = append(errs, coreconfig.Required(path.Child("kind")))
	}
	if r.Name == "" {
		errs = append(errs, coreconfig.Required(path.Child("name")))
	}
	return errs
}
