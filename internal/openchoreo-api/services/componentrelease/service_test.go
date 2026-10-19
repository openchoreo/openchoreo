// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace     = "test-ns"
	testProjectName   = "test-project"
	testComponentName = "test-comp"
	testReleaseName   = "test-release"
)

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

func TestListComponentReleases(t *testing.T) {
	ctx := context.Background()

	t.Run("all without component filter", func(t *testing.T) {
		cr1 := testutil.NewComponentRelease(testNamespace, testProjectName, "comp-a", "rel-1")
		cr2 := testutil.NewComponentRelease(testNamespace, testProjectName, "comp-b", "rel-2")
		svc := newService(t, cr1, cr2)

		result, err := svc.ListComponentReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, componentReleaseTypeMeta, item.TypeMeta)
		}
	})

	t.Run("with component filter", func(t *testing.T) {
		cr1 := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, "rel-1")
		cr2 := testutil.NewComponentRelease(testNamespace, testProjectName, "other-comp", "rel-2")
		svc := newService(t, cr1, cr2)

		result, err := svc.ListComponentReleases(ctx, testNamespace, testComponentName, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testComponentName, result.Items[0].Spec.Owner.ComponentName)
	})

	t.Run("filtered path with limit", func(t *testing.T) {
		objs := make([]client.Object, 0, 5)
		for i := range 5 {
			cr := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, fmt.Sprintf("rel-%d", i))
			objs = append(objs, cr)
		}
		svc := newService(t, objs...)

		result, err := svc.ListComponentReleases(ctx, testNamespace, testComponentName, services.ListOptions{Limit: 2})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(result.Items), 2)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListComponentReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListComponentReleases(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		crInNs := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, "rel-in")
		crOtherNs := testutil.NewComponentRelease("other-ns", testProjectName, testComponentName, "rel-out")
		svc := newService(t, crInNs, crOtherNs)

		result, err := svc.ListComponentReleases(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "rel-in", result.Items[0].Name)
	})
}

func TestGetComponentRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		cr := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, testReleaseName)
		svc := newService(t, cr)

		result, err := svc.GetComponentRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)
		assert.Equal(t, componentReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, testReleaseName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetComponentRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentReleaseNotFound)
	})
}

func TestCreateComponentRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		cr := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: testReleaseName},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
				},
				ComponentType: openchoreov1alpha1.ComponentReleaseComponentType{
					Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
					Name: "deployment/test-type",
					Spec: openchoreov1alpha1.ComponentTypeSpec{WorkloadType: "deployment"},
				},
			},
		}

		result, err := svc.CreateComponentRelease(ctx, testNamespace, cr)
		require.NoError(t, err)
		assert.Equal(t, componentReleaseTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, openchoreov1alpha1.ComponentReleaseStatus{}, result.Status)
	})

	lineRelease := func() *openchoreov1alpha1.ComponentRelease {
		return &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: testReleaseName},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
					ReleaseLine:   "next",
				},
			},
		}
	}
	lineObjects := func(policy openchoreov1alpha1.ReleaseLinePolicy) []client.Object {
		return []client.Object{
			&openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{Name: testComponentName, Namespace: testNamespace},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{ProjectName: testProjectName},
					ComponentType: openchoreov1alpha1.ComponentTypeRef{
						Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
						Name: "deployment/line-type",
					},
					ReleaseLinesEnabled: true,
				},
			},
			&openchoreov1alpha1.ClusterComponentType{
				ObjectMeta: metav1.ObjectMeta{Name: "line-type"},
				Spec:       openchoreov1alpha1.ClusterComponentTypeSpec{WorkloadType: "deployment", ReleaseLines: policy},
			},
			&openchoreov1alpha1.ReleaseLine{
				ObjectMeta: metav1.ObjectMeta{Name: "next", Namespace: testNamespace},
				Spec: openchoreov1alpha1.ReleaseLineSpec{
					Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: testProjectName, ComponentName: testComponentName},
				},
			},
		}
	}
	requireValidationError := func(t *testing.T, err error, msg string) {
		t.Helper()
		var vErr *services.ValidationError
		require.ErrorAs(t, err, &vErr)
		assert.Contains(t, vErr.Msg, msg)
	}

	t.Run("line release for an unknown line", func(t *testing.T) {
		svc := newService(t, lineObjects(openchoreov1alpha1.ReleaseLinePolicyAllowed)...)
		cr := lineRelease()
		cr.Spec.Owner.ReleaseLine = "v9"
		_, err := svc.CreateComponentRelease(ctx, testNamespace, cr)
		requireValidationError(t, err, `release line "v9" not found for component "test-comp"`)
	})

	t.Run("release without a line for a component with release lines", func(t *testing.T) {
		svc := newService(t, lineObjects(openchoreov1alpha1.ReleaseLinePolicyAllowed)...)
		cr := lineRelease()
		cr.Spec.Owner.ReleaseLine = ""
		_, err := svc.CreateComponentRelease(ctx, testNamespace, cr)
		requireValidationError(t, err, "a release line is required")
	})

	t.Run("line release on a type that allows release lines", func(t *testing.T) {
		svc := newService(t, lineObjects(openchoreov1alpha1.ReleaseLinePolicyAllowed)...)
		_, err := svc.CreateComponentRelease(ctx, testNamespace, lineRelease())
		require.NoError(t, err)
	})

	t.Run("line release on a type that does not allow release lines", func(t *testing.T) {
		svc := newService(t, lineObjects("")...)
		_, err := svc.CreateComponentRelease(ctx, testNamespace, lineRelease())
		requireValidationError(t, err, `component type "deployment/line-type" does not allow release lines`)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateComponentRelease(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrComponentReleaseNil)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, testReleaseName)
		svc := newService(t, existing)
		cr := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{Name: testReleaseName},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   testProjectName,
					ComponentName: testComponentName,
				},
			},
		}

		_, err := svc.CreateComponentRelease(ctx, testNamespace, cr)
		require.ErrorIs(t, err, ErrComponentReleaseAlreadyExists)
	})
}

func TestDeleteComponentRelease(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		cr := testutil.NewComponentRelease(testNamespace, testProjectName, testComponentName, testReleaseName)
		svc := newService(t, cr)

		err := svc.DeleteComponentRelease(ctx, testNamespace, testReleaseName)
		require.NoError(t, err)

		_, err = svc.GetComponentRelease(ctx, testNamespace, testReleaseName)
		require.ErrorIs(t, err, ErrComponentReleaseNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteComponentRelease(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrComponentReleaseNotFound)
	})
}
