// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace     = "test-ns"
	testProjectName   = "test-project"
	testComponentName = "test-component"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func testComponent() *openchoreov1alpha1.Component {
	return testutil.NewComponent(testNamespace, testProjectName, testComponentName)
}

func linesEnabledComponent() *openchoreov1alpha1.Component {
	comp := testComponent()
	comp.Spec.ReleaseLinesEnabled = true
	return comp
}

func testReleaseLine(name, project, component string) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: component},
		},
	}
}

func rawParams(raw string) *runtime.RawExtension {
	return &runtime.RawExtension{Raw: []byte(raw)}
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
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("next", testProjectName, "other-comp"))
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Contains(t, vErr.Msg, "spec.owner.componentName")
	})

	t.Run("owner naming another project", func(t *testing.T) {
		svc := newService(t, linesEnabledComponent())
		_, err := svc.CreateReleaseLine(ctx, testNamespace, testComponentName, testReleaseLine("next", "other-proj", testComponentName))
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
		rl.Spec.Parameters = rawParams(`{"logLevel":"debug"}`)
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
