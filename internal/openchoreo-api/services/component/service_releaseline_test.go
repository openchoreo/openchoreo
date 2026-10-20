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
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
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
	comp.Spec.ReleaseLinesEnabled = true
	comp.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "info", "replicas": 1})

	next := testReleaseLine("next", testProjectName, testComponentName)
	next.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "debug"})

	mainWorkload := testWorkload()
	mainWorkload.Name = testComponentName + "-main-workload"
	mainWorkload.Spec.Owner.ReleaseLine = "main"
	mainWorkload.Spec.Container.Image = "app:main"

	nextWorkload := testWorkload()
	nextWorkload.Name = testComponentName + "-next-workload"
	nextWorkload.Spec.Owner.ReleaseLine = "next"
	nextWorkload.Spec.Container.Image = "app:next"

	return []client.Object{
		testProject(), testDeploymentPipeline(), comp, lineComponentType(),
		testReleaseLine("main", testProjectName, testComponentName), next,
		mainWorkload, nextWorkload,
	}
}

// lineComponentType is the test component type with release lines allowed.
func lineComponentType() *openchoreov1alpha1.ComponentType {
	ct := testComponentType()
	ct.Spec.ReleaseLines = openchoreov1alpha1.ReleaseLinePolicyAllowed
	return ct
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

	t.Run("line required when enabled", func(t *testing.T) {
		svc := newService(t, lineSeedObjects(t)...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{})
		require.ErrorIs(t, err, ErrReleaseLineRequired)
	})

	t.Run("unknown line", func(t *testing.T) {
		svc := newService(t, lineSeedObjects(t)...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "v9"})
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})

	t.Run("line rejected when the component type does not allow lines", func(t *testing.T) {
		objs := lineSeedObjects(t)
		for i, obj := range objs {
			if _, ok := obj.(*openchoreov1alpha1.ComponentType); ok {
				objs[i] = testComponentType()
			}
		}
		svc := newService(t, objs...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "next"})
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Contains(t, vErr.Msg, "does not allow release lines")
	})

	t.Run("line rejected when not enabled", func(t *testing.T) {
		svc := newService(t, tier3SeedObjects()...)
		_, err := svc.GenerateRelease(ctx, testNamespace, testComponentName, &GenerateReleaseRequest{ReleaseLine: "next"})
		require.ErrorIs(t, err, ErrReleaseLinesNotEnabled)
	})
}
