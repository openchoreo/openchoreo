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

func TestListReleaseLines(t *testing.T) {
	ctx := context.Background()

	t.Run("returns only the component's lines", func(t *testing.T) {
		svc := newService(t,
			testComponent(),
			testReleaseLine("next", testProjectName, testComponentName),
			testReleaseLine("main", testProjectName, testComponentName),
			testReleaseLine("other-comp", testProjectName, "other"),
			testReleaseLine("other-proj", "other", testComponentName),
		)
		result, err := svc.ListReleaseLines(ctx, testNamespace, testComponentName, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		require.Len(t, result.Items, 2)
		assert.ElementsMatch(t, []string{"main", "next"}, []string{result.Items[0].Name, result.Items[1].Name})
		assert.Equal(t, releaseLineTypeMeta, result.Items[0].TypeMeta)
	})

	t.Run("component without lines", func(t *testing.T) {
		svc := newService(t, testComponent())
		result, err := svc.ListReleaseLines(ctx, testNamespace, testComponentName, services.ListOptions{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("component not found", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.ListReleaseLines(ctx, testNamespace, "nonexistent", services.ListOptions{Limit: 10})
		require.ErrorIs(t, err, ErrComponentNotFound)
	})
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

func linesEnabledComponent() *openchoreov1alpha1.Component {
	comp := testComponent()
	comp.Spec.ReleaseLinesEnabled = true
	return comp
}

func TestGetReleaseLine(t *testing.T) {
	ctx := context.Background()
	svc := newService(t,
		linesEnabledComponent(),
		testReleaseLine("next", testProjectName, testComponentName),
		testReleaseLine("other", testProjectName, "other-comp"),
	)

	t.Run("returns the component's line", func(t *testing.T) {
		rl, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "next")
		require.NoError(t, err)
		assert.Equal(t, "next", rl.Name)
		assert.Equal(t, releaseLineTypeMeta, rl.TypeMeta)
	})

	t.Run("line of another component", func(t *testing.T) {
		_, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "other")
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})

	t.Run("unknown line", func(t *testing.T) {
		_, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "missing")
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})

	t.Run("unknown component", func(t *testing.T) {
		_, err := svc.GetReleaseLine(ctx, testNamespace, "missing", "next")
		require.ErrorIs(t, err, ErrComponentNotFound)
	})
}

func TestCreateReleaseLine(t *testing.T) {
	ctx := context.Background()

	t.Run("fills the owner from the component", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent())
		rl := &openchoreov1alpha1.ReleaseLine{ObjectMeta: metav1.ObjectMeta{Name: "next"}}
		created, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, rl)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, created.Namespace)
		assert.Equal(t, openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProjectName, ComponentName: testComponentName}, created.Spec.Owner)
		assert.Equal(t, releaseLineTypeMeta, created.TypeMeta)

		got, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "next")
		require.NoError(t, err)
		assert.Equal(t, testComponentName, got.Spec.Owner.ComponentName)
	})

	t.Run("owner naming another component", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent())
		rl := testReleaseLine("next", testProjectName, "other-comp")
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, rl)
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Contains(t, vErr.Msg, "spec.owner.componentName")
	})

	t.Run("owner naming another project", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent())
		rl := testReleaseLine("next", "other-proj", testComponentName)
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, rl)
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Contains(t, vErr.Msg, "spec.owner.projectName")
	})

	t.Run("component without release lines", func(t *testing.T) {
		svc := newService(t, testComponent())
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("next", "", ""))
		require.ErrorIs(t, err, ErrReleaseLinesNotEnabled)
	})

	t.Run("name taken by another component's line", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("next", testProjectName, "other-comp"))
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("next", "", ""))
		require.ErrorIs(t, err, ErrReleaseLineAlreadyExists)
	})

	t.Run("unknown component", func(t *testing.T) {
		svc := newService(t)
		_, err := svc.CreateReleaseLine(ctx, testNamespace, "missing", testReleaseLine("next", "", ""))
		require.ErrorIs(t, err, ErrComponentNotFound)
	})
}

func TestUpdateReleaseLine(t *testing.T) {
	ctx := context.Background()

	t.Run("replaces the spec and keeps the owner", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("next", testProjectName, testComponentName))
		rl := &openchoreov1alpha1.ReleaseLine{ObjectMeta: metav1.ObjectMeta{Name: "next"}}
		rl.Spec.Parameters = rawJSON(t, map[string]any{"logLevel": "debug"})
		updated, err := svc.UpdateReleaseLine(ctx, testNamespace, testComponentName, rl)
		require.NoError(t, err)
		assert.JSONEq(t, `{"logLevel":"debug"}`, string(updated.Spec.Parameters.Raw))
		assert.Equal(t, testComponentName, updated.Spec.Owner.ComponentName)

		got, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "next")
		require.NoError(t, err)
		assert.JSONEq(t, `{"logLevel":"debug"}`, string(got.Spec.Parameters.Raw))
	})

	t.Run("owner naming another component", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("next", testProjectName, testComponentName))
		_, err := svc.UpdateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("next", testProjectName, "other-comp"))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
	})

	t.Run("line of another component", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("other", testProjectName, "other-comp"))
		_, err := svc.UpdateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("other", "", ""))
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})
}

func TestDeleteReleaseLine(t *testing.T) {
	ctx := context.Background()

	t.Run("deletes the component's line", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("next", testProjectName, testComponentName))
		require.NoError(t, svc.DeleteReleaseLine(ctx, testNamespace, testComponentName, "next"))
		_, err := svc.GetReleaseLine(ctx, testNamespace, testComponentName, "next")
		require.ErrorIs(t, err, ErrReleaseLineNotFound)
	})

	t.Run("line of another component is kept", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent(), testReleaseLine("other", testProjectName, "other-comp"))
		require.ErrorIs(t, svc.DeleteReleaseLine(ctx, testNamespace, testComponentName, "other"), ErrReleaseLineNotFound)
	})

	t.Run("unknown line", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent())
		require.ErrorIs(t, svc.DeleteReleaseLine(ctx, testNamespace, testComponentName, "missing"), ErrReleaseLineNotFound)
	})
}
