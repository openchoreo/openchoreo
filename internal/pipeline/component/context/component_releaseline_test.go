// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

func TestBuildComponentContext_DerivedNamesFollowReleaseLineName(t *testing.T) {
	workload := &v1alpha1.Workload{Spec: v1alpha1.WorkloadSpec{WorkloadTemplateSpec: v1alpha1.WorkloadTemplateSpec{
		Container: v1alpha1.Container{Image: "app:1", Env: []v1alpha1.EnvVar{{Key: "MODE", Value: "x"}}},
	}}}

	build := func(releaseLineName string) *ComponentContext {
		t.Helper()
		ctx, err := BuildComponentContext(&ComponentContextInput{
			Component:      &v1alpha1.Component{},
			ComponentType:  &v1alpha1.ComponentType{},
			DataPlane:      &v1alpha1.DataPlane{},
			Environment:    &v1alpha1.Environment{},
			WorkloadData:   ExtractWorkloadData(workload),
			Configurations: ExtractConfigurationsFromWorkload(nil, workload),
			Metadata: MetadataContext{
				Name:               "myapp-dev-12345678",
				Namespace:          "dp-test-ns",
				ComponentName:      "myapp",
				ReleaseLineName:    releaseLineName,
				ComponentUID:       "uid-1234",
				ComponentNamespace: "cp-test-ns",
				ProjectName:        "test-project",
				ProjectUID:         "proj-uid",
				DataPlaneName:      "test-dp",
				DataPlaneUID:       "dp-uid",
				EnvironmentName:    "dev",
				EnvironmentUID:     "env-uid",
				Labels:             map[string]string{},
				Annotations:        map[string]string{},
				PodSelectors:       map[string]string{"openchoreo.dev/component-uid": "uid-1234"},
			},
		})
		require.NoError(t, err)
		return ctx
	}

	unlined := build("")
	assert.Equal(t, "myapp", unlined.Metadata.ReleaseLineName)
	require.Len(t, unlined.Derived.ContainerEnvFrom, 1)
	assert.Equal(t, generateEnvResourceName("myapp-dev"), unlined.Derived.ContainerEnvFrom[0].ConfigMapRef.Name)

	line := build("myapp-v2")
	require.Len(t, line.Derived.ContainerEnvFrom, 1)
	assert.Equal(t, generateEnvResourceName("myapp-v2-dev"), line.Derived.ContainerEnvFrom[0].ConfigMapRef.Name)
}
