// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func testReleaseLine(name, project, component string) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: component},
		},
	}
}

func lineSeedObjects(t *testing.T) []client.Object {
	comp := testComponent()
	comp.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "info", "replicas": 1})

	next := testReleaseLine("next", testProjectName, testComponentName)
	next.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "debug"})

	mainWorkload := testWorkload()
	mainWorkload.Name = testComponentName + "-workload"
	mainWorkload.Spec.Container.Image = "app:main"

	nextWorkload := testWorkload()
	nextWorkload.Name = testComponentName + "-next-workload"
	nextWorkload.Spec.Owner.ReleaseLine = "next"
	nextWorkload.Spec.Container.Image = "app:next"

	return []client.Object{
		testProject(), testDeploymentPipeline(), comp, testComponentType(),
		next, mainWorkload, nextWorkload,
	}
}

func TestGenerateRelease_ReleaseLine(t *testing.T) {
	ctx := context.Background()

	t.Run("builds from the line's workload and overrides", func(t *testing.T) {
		svc := newService(t, lineSeedObjects(t)...)
		result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "next"})
		require.NoError(t, err)

		assert.True(t, strings.HasPrefix(result.Name, testComponentName+"-next-"), result.Name)
		assert.Equal(t, "next", result.Spec.Owner.ReleaseLine)
		assert.Equal(t, "next", result.Labels[labels.LabelKeyReleaseLine])
		assert.Equal(t, "app:next", result.Spec.Workload.Container.Image)
		require.NotNil(t, result.Spec.ComponentProfile)
		var params map[string]any
		require.NoError(t, json.Unmarshal(result.Spec.ComponentProfile.Parameters.Raw, &params))
		assert.Equal(t, map[string]any{"logLevel": "debug", "replicas": float64(1)}, params)
	})

	for name, line := range map[string]string{"no line": "", "main line keyword": "main"} {
		t.Run(name+" builds the main line", func(t *testing.T) {
			svc := newService(t, lineSeedObjects(t)...)
			result, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: line})
			require.NoError(t, err)

			assert.False(t, strings.HasPrefix(result.Name, testComponentName+"-next-"), result.Name)
			assert.Empty(t, result.Spec.Owner.ReleaseLine)
			assert.NotContains(t, result.Labels, labels.LabelKeyReleaseLine)
			assert.Equal(t, "app:main", result.Spec.Workload.Container.Image)
		})
	}

	t.Run("unknown line", func(t *testing.T) {
		svc := newService(t, lineSeedObjects(t)...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "v9"})
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})

	t.Run("line rejected on a component without it", func(t *testing.T) {
		svc := newService(t, tier3SeedObjects()...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "next"})
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})
}
