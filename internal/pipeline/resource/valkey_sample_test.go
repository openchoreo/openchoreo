// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcepipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// loadValkeySampleSpec reads the getting-started valkey ClusterResourceType
// sample and returns its Spec as a ResourceTypeSpec. ClusterResourceTypeSpec
// and ResourceTypeSpec are structurally identical (see
// api/v1alpha1/clusterresourcetype_types.go), so a JSON round-trip is a
// faithful conversion for exercising the sample through this package's
// ResourceType-shaped RenderInput.
func loadValkeySampleSpec(t *testing.T) v1alpha1.ResourceTypeSpec {
	t.Helper()

	path := filepath.Join("..", "..", "..", "samples", "getting-started", "cluster-resource-types", "valkey.yaml")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	var crt v1alpha1.ClusterResourceType
	require.NoError(t, yaml.Unmarshal(raw, &crt))

	b, err := json.Marshal(crt.Spec)
	require.NoError(t, err)

	var spec v1alpha1.ResourceTypeSpec
	require.NoError(t, json.Unmarshal(b, &spec))
	return spec
}

// TestValkeySample_RendersWithoutGateway is a regression test for a
// getting-started sample bug: the valkey ClusterResourceType referenced the
// top-level ${gateway.*} alias directly. When a DataPlane has no gateway
// configured (spec.gateway: {}), the "gateway" key is entirely absent from
// the CEL context (it's an omitempty pointer), so the top-level "gateway"
// CEL variable is never declared and any expression mentioning it fails to
// *compile* -- regardless of which branch would actually run, since
// adminEnabled=false only short-circuits at evaluation time, after CEL has
// already type-checked the whole expression. This reproduces that scenario
// (both DataPlane and Environment left zero-valued, i.e. no gateway) and
// asserts the sample renders cleanly with the admin resources skipped.
func TestValkeySample_RendersWithoutGateway(t *testing.T) {
	spec := loadValkeySampleSpec(t)

	// DataPlane and Environment are left zero-valued: no gateway configured
	// anywhere, matching "spec.gateway: {}" from the issue.
	input := makeRenderInput(spec)

	got, err := NewPipeline().RenderManifests(input)
	require.NoError(t, err, "rendering must not fail with a CEL compile error when no gateway is configured")

	gotIDs := make([]string, 0, len(got.Entries))
	for _, e := range got.Entries {
		gotIDs = append(gotIDs, e.ID)
	}
	require.ElementsMatch(t, []string{"password-generator", "creds", "service", "statefulset"}, gotIDs,
		"admin-* resources must be excluded when adminEnabled defaults to false")

	resolved, err := NewPipeline().ResolveOutputs(input, nil)
	require.NoError(t, err)

	outputs := make(map[string]any, len(resolved))
	for _, ro := range resolved {
		outputs[ro.Name] = ro.Value
	}
	require.Equal(t, "disabled", outputs["adminURL"], "adminURL must resolve to disabled, not error out, when adminEnabled is false")
}
