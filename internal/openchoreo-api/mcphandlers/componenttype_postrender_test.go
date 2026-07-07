// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package mcphandlers

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// TestConvertSpec_ComponentTypePostRenderValidationsRoundTrip proves a ComponentType
// postRenderValidations entry survives the gen <-> CRD JSON round-trip used by the MCP
// handlers, so the API authoring surface does not silently drop the field.
func TestConvertSpec_ComponentTypePostRenderValidationsRoundTrip(t *testing.T) {
	orig := openchoreov1alpha1.ComponentTypeSpec{
		WorkloadType: "deployment",
		PreRenderValidations: []openchoreov1alpha1.ValidationRule{
			{Rule: "${1 == 1}", Message: "pre"},
		},
		PostRenderValidations: []openchoreov1alpha1.PostRenderValidation{{
			Target:  openchoreov1alpha1.PostRenderTarget{PatchTarget: openchoreov1alpha1.PatchTarget{Group: "apps", Version: "v1", Kind: "Deployment"}},
			Rule:    "${resource.spec.replicas == 1}",
			Message: "single replica",
		}},
	}

	// Marshal the CRD spec and load it into the generated model, mimicking an inbound
	// API request body, then convert back to the CRD spec as the handlers do.
	data, err := json.Marshal(orig)
	require.NoError(t, err)
	var genSpec gen.ComponentTypeSpec
	require.NoError(t, json.Unmarshal(data, &genSpec))

	got, err := convertSpec[gen.ComponentTypeSpec, openchoreov1alpha1.ComponentTypeSpec](genSpec)
	require.NoError(t, err)

	require.Len(t, got.PreRenderValidations, 1)
	require.Equal(t, "pre", got.PreRenderValidations[0].Message)
	require.Len(t, got.PostRenderValidations, 1)
	require.Equal(t, "single replica", got.PostRenderValidations[0].Message)
	require.Equal(t, "Deployment", got.PostRenderValidations[0].Target.Kind)
}
