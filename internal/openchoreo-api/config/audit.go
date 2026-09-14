// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/openchoreo/openchoreo/internal/auditconfig"
)

// AuditConfig defines audit logging settings. A type alias onto the shared
// internal/auditconfig library — see that package for the koanf-decode-and-
// validate logic. openchoreo-api's own vocabulary (which resources/operations/
// actions a policy selector may name) is supplied at each call site via
// auditconfig.NewVocabulary(apiaudit.GetOperations()), not baked in here.
type (
	// AuditConfig is auditconfig.AuditConfig — see the package comment above.
	AuditConfig = auditconfig.AuditConfig
	// PolicyDefaultsConfig is auditconfig.PolicyDefaultsConfig.
	PolicyDefaultsConfig = auditconfig.PolicyDefaultsConfig
	// PolicyRuleConfig is auditconfig.PolicyRuleConfig.
	PolicyRuleConfig = auditconfig.PolicyRuleConfig
	// SelectorConfig is auditconfig.SelectorConfig.
	SelectorConfig = auditconfig.SelectorConfig
)

// AuditDefaults returns the default audit configuration.
func AuditDefaults() AuditConfig {
	return auditconfig.AuditDefaults()
}
