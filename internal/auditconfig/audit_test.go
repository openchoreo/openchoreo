// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package auditconfig_test

import (
	"testing"

	"github.com/openchoreo/openchoreo/internal/auditconfig"
	"github.com/openchoreo/openchoreo/internal/config"
	apiaudit "github.com/openchoreo/openchoreo/internal/openchoreo-api/audit"
	"github.com/openchoreo/openchoreo/internal/server/middleware/audit"
)

func TestValidate_AccessCategoryIsSelectable(t *testing.T) {
	cfg := auditconfig.AuditDefaults()
	cfg.Defaults.Publish = false
	cfg.Policies = []auditconfig.PolicyRuleConfig{{
		Match: auditconfig.SelectorConfig{Categories: []string{string(audit.CategoryAccess)}},
		Set:   map[string]any{"publish": true},
	}}
	vocab := auditconfig.NewVocabulary(apiaudit.GetOperations())
	if errs := cfg.Validate(config.NewPath("audit"), vocab, []string{"user"}); len(errs) > 0 {
		t.Errorf("categories: [access] was rejected: %v", errs)
	}
}
