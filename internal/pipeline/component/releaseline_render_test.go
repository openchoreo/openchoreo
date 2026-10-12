// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
)

// TestPipeline_Render_ReleaseLineServiceTypeLinesDoNotCollide renders the release line sample's
// service ComponentType for two release lines of one component and checks that they share the
// project namespace without sharing any object name, gateway path, or pod selector.
func TestPipeline_Render_ReleaseLineServiceTypeLinesDoNotCollide(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "samples", "release-lines", "component-type.yaml"))
	require.NoError(t, err)
	var cct v1alpha1.ClusterComponentType
	require.NoError(t, yaml.Unmarshal(raw, &cct))
	ctSpec := cct.Spec.ToComponentTypeSpec()

	var environment v1alpha1.Environment
	require.NoError(t, yaml.Unmarshal([]byte(`
metadata: {name: dev, namespace: cp-ns}
spec:
  dataPlaneRef: {kind: DataPlane, name: dp}`), &environment))
	var dataPlane v1alpha1.DataPlane
	require.NoError(t, yaml.Unmarshal([]byte(`
metadata: {name: dp, namespace: cp-ns}
spec:
  gateway:
    ingress:
      external:
        name: gateway-default
        namespace: gateway-ns
        http: {port: 80, host: api.example.com}
      internal:
        name: gateway-default
        namespace: gateway-ns
        http: {port: 80, host: internal.example.com}`), &dataPlane))

	workload := &v1alpha1.Workload{Spec: v1alpha1.WorkloadSpec{WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
		Container: v1alpha1.Container{Image: "app:1", Env: []v1alpha1.EnvVar{{Key: "MODE", Value: "x"}}},
		Endpoints: map[string]v1alpha1.WorkloadEndpoint{
			"http": {Type: v1alpha1.EndpointTypeHTTP, Port: 8080, Visibility: []v1alpha1.EndpointVisibility{
				v1alpha1.EndpointVisibilityExternal, v1alpha1.EndpointVisibilityInternal,
			}},
		},
	}}}

	render := func(line string) []map[string]any {
		t.Helper()
		selectors := map[string]string{"openchoreo.dev/component-uid": "comp-uid", "openchoreo.dev/release-line": line}
		output, err := NewPipeline().Render(t.Context(), &RenderInput{
			ComponentType: &v1alpha1.ComponentType{Spec: ctSpec},
			Component:     &v1alpha1.Component{},
			Workload:      workload,
			Environment:   &environment,
			DataPlane:     &dataPlane,
			Metadata: context.MetadataContext{
				Name:               "orders-" + line + "-dev-12345678",
				Namespace:          "dp-cp-ns-proj-dev-12345678",
				ComponentName:      "orders",
				ReleaseLineName:    "orders-" + line,
				ReleaseLine:        line,
				ComponentUID:       "comp-uid",
				ComponentNamespace: "cp-ns",
				ProjectName:        "proj",
				ProjectUID:         "proj-uid",
				DataPlaneName:      "dp",
				DataPlaneUID:       "dp-uid",
				EnvironmentName:    "dev",
				EnvironmentUID:     "env-uid",
				Labels:             selectors,
				Annotations:        map[string]string{},
				PodSelectors:       selectors,
			},
		})
		require.NoError(t, err)
		resources := make([]map[string]any, 0, len(output.Resources))
		for _, rr := range output.Resources {
			resources = append(resources, rr.Resource)
		}
		return resources
	}

	identities := func(resources []map[string]any) map[string]bool {
		ids := map[string]bool{}
		for _, r := range resources {
			md := r["metadata"].(map[string]any)
			ids[fmt.Sprintf("%s %s/%s", r["kind"], md["namespace"], md["name"])] = true
		}
		return ids
	}
	find := func(resources []map[string]any, kind string) []map[string]any {
		var out []map[string]any
		for _, r := range resources {
			if r["kind"] == kind {
				out = append(out, r)
			}
		}
		return out
	}

	mainLine, v2 := render("main"), render("v2")

	kinds := map[string]bool{}
	for _, r := range mainLine {
		kinds[r["kind"].(string)] = true
	}
	for _, kind := range []string{"Deployment", "Service", "HTTPRoute", "ConfigMap"} {
		assert.True(t, kinds[kind], "expected the service type to render a %s", kind)
	}

	mainIDs := identities(mainLine)
	for id := range identities(v2) {
		assert.False(t, mainIDs[id], "both release lines render %s", id)
	}

	mainSvc, v2Svc := find(mainLine, "Service")[0], find(v2, "Service")[0]
	assert.NotEqual(t, mainSvc["spec"].(map[string]any)["selector"], v2Svc["spec"].(map[string]any)["selector"])

	pathsOf := func(resources []map[string]any) map[string]bool {
		paths := map[string]bool{}
		for _, route := range find(resources, "HTTPRoute") {
			for _, rule := range route["spec"].(map[string]any)["rules"].([]any) {
				for _, m := range rule.(map[string]any)["matches"].([]any) {
					paths[fmt.Sprint(m.(map[string]any)["path"].(map[string]any)["value"])] = true
				}
			}
		}
		return paths
	}
	mainPaths := pathsOf(mainLine)
	for path := range pathsOf(v2) {
		assert.False(t, mainPaths[path], "both release lines route %s", path)
	}
}
