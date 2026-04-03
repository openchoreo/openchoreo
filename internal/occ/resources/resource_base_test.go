// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
)

func TestNewResourceBase_NoOptions(t *testing.T) {
	base := NewResourceBase()
	assert.NotNil(t, base)
	assert.Empty(t, base.GetNamespace())
	assert.Equal(t, constants.CRDConfig{}, base.GetConfig())
}

func TestNewResourceBase_WithNamespace(t *testing.T) {
	base := NewResourceBase(WithResourceNamespace("my-ns"))
	assert.Equal(t, "my-ns", base.GetNamespace())
}

func TestNewResourceBase_WithConfig(t *testing.T) {
	cfg := constants.CRDConfig{
		Group:   "openchoreo.dev",
		Version: constants.V1Alpha1,
		Kind:    "Workload",
	}
	base := NewResourceBase(WithResourceConfig(cfg))
	assert.Equal(t, cfg, base.GetConfig())
}

func TestNewResourceBase_MultipleOptions(t *testing.T) {
	cfg := constants.CRDConfig{
		Group:   "openchoreo.dev",
		Version: constants.V1Alpha1,
		Kind:    "Component",
	}
	base := NewResourceBase(
		WithResourceNamespace("prod"),
		WithResourceConfig(cfg),
	)
	assert.Equal(t, "prod", base.GetNamespace())
	assert.Equal(t, cfg, base.GetConfig())
}

func TestResourceBase_SetNamespace(t *testing.T) {
	base := NewResourceBase(WithResourceNamespace("old-ns"))
	assert.Equal(t, "old-ns", base.GetNamespace())

	base.SetNamespace("new-ns")
	assert.Equal(t, "new-ns", base.GetNamespace())
}

func TestResourceBase_SetNamespace_FromEmpty(t *testing.T) {
	base := NewResourceBase()
	assert.Empty(t, base.GetNamespace())

	base.SetNamespace("my-ns")
	assert.Equal(t, "my-ns", base.GetNamespace())
}

func TestWithResourceNamespace_EmptyString(t *testing.T) {
	base := NewResourceBase(WithResourceNamespace(""))
	assert.Empty(t, base.GetNamespace())
}

func TestWithResourceConfig_ZeroValue(t *testing.T) {
	base := NewResourceBase(WithResourceConfig(constants.CRDConfig{}))
	assert.Equal(t, constants.CRDConfig{}, base.GetConfig())
}
