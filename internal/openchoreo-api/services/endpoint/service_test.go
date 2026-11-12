// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	projectsvc "github.com/openchoreo/openchoreo/internal/openchoreo-api/services/project"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

const (
	testNamespace = "test-ns"
	testProject   = "test-project"
	testComponent = "greeter"
)

// newService creates a Endpoint service plus the project fixture it depends on
// for the project-existence check on Create / List.
func newService(t *testing.T, extra ...client.Object) Service {
	t.Helper()
	objs := make([]client.Object, 0, 1+len(extra))
	objs = append(objs, testutil.NewProject(testNamespace, testProject))
	objs = append(objs, extra...)
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

const testLine = "next"

func newReleaseLine(project, component, line string) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{Name: line, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: project, ComponentName: component},
		},
	}
}

// newLineEndpoint returns the Endpoint "test-r", whose backend routes to testLine of
// testComponent and whose spec.owner names that line.
func newLineEndpoint() *openchoreov1alpha1.Endpoint {
	ep := testutil.NewEndpoint(testNamespace, testProject, "test-r")
	ep.Spec.BackendRefs[0].Component.ReleaseLine = testLine
	ep.Spec.Owner.ComponentName = testComponent
	ep.Spec.Owner.ReleaseLineName = testLine
	return ep
}

func TestEndpointReleaseLineOwner(t *testing.T) {
	ctx := context.Background()
	fixtures := func() []client.Object {
		return []client.Object{
			testutil.NewComponent(testNamespace, testProject, testComponent),
			newReleaseLine(testProject, testComponent, testLine),
		}
	}

	t.Run("created with the line its backends route to", func(t *testing.T) {
		svc := newService(t, fixtures()...)

		result, err := svc.CreateEndpoint(ctx, testNamespace, newLineEndpoint())
		require.NoError(t, err)
		assert.Equal(t, testComponent, result.Spec.Owner.ComponentName)
		assert.Equal(t, testLine, result.Spec.Owner.ReleaseLineName)
	})

	t.Run("a backend on the main line moves ownership to the component", func(t *testing.T) {
		existing := newLineEndpoint()
		svc := newService(t, append(fixtures(), existing)...)

		update := existing.DeepCopy()
		update.Spec.BackendRefs = append(update.Spec.BackendRefs, openchoreov1alpha1.EndpointBackendRef{
			Name: "main", Kind: openchoreov1alpha1.EndpointBackendKindComponent,
			Component: &openchoreov1alpha1.EndpointComponentBackend{Name: testComponent, Endpoint: "http"},
		})

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, testComponent, result.Spec.Owner.ComponentName)
		assert.Empty(t, result.Spec.Owner.ReleaseLineName)
	})

	t.Run("a backend on another component moves ownership to the project", func(t *testing.T) {
		existing := newLineEndpoint()
		svc := newService(t, append(fixtures(), existing)...)

		update := existing.DeepCopy()
		update.Spec.BackendRefs[0].Component.Name = "billing"

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Empty(t, result.Spec.Owner.ComponentName)
		assert.Empty(t, result.Spec.Owner.ReleaseLineName)
	})

	t.Run("a line without a component is dropped", func(t *testing.T) {
		svc := newService(t, fixtures()...)
		ep := newLineEndpoint()
		ep.Spec.Owner.ComponentName = ""

		result, err := svc.CreateEndpoint(ctx, testNamespace, ep)
		require.NoError(t, err)
		assert.Empty(t, result.Spec.Owner.ReleaseLineName)
	})

	t.Run("owning line not found", func(t *testing.T) {
		svc := newService(t, testutil.NewComponent(testNamespace, testProject, testComponent))

		_, err := svc.CreateEndpoint(ctx, testNamespace, newLineEndpoint())
		var validationErr *services.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, `release line "next" not found`)
	})

	t.Run("owning line of another component", func(t *testing.T) {
		svc := newService(t,
			testutil.NewComponent(testNamespace, testProject, testComponent),
			newReleaseLine(testProject, "billing", testLine))

		_, err := svc.CreateEndpoint(ctx, testNamespace, newLineEndpoint())
		var validationErr *services.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, `belongs to component "billing"`)
	})
}

func TestCreateEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r", result.Name)
		assert.Equal(t, testNamespace, result.Namespace)
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.CreateEndpoint(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("project not found", func(t *testing.T) {
		svc := NewService(testutil.NewFakeClient(), testutil.TestLogger())
		r := testutil.NewEndpoint(testNamespace, "missing-project", "test-r")

		_, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "dup")
		svc := newService(t, existing)
		dup := testutil.NewEndpoint(testNamespace, testProject, "dup")

		_, err := svc.CreateEndpoint(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrEndpointAlreadyExists)
	})

	t.Run("owned by the component its backends route to", func(t *testing.T) {
		svc := newService(t, testutil.NewComponent(testNamespace, testProject, testComponent))
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		r.Spec.Owner.ComponentName = testComponent

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Equal(t, testComponent, result.Spec.Owner.ComponentName)
	})

	t.Run("owned by the project when a backend routes to another component", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		r.Spec.Owner.ComponentName = "not-a-backend"

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Empty(t, result.Spec.Owner.ComponentName)
	})

	t.Run("owning component not found", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		r.Spec.Owner.ComponentName = testComponent

		_, err := svc.CreateEndpoint(ctx, testNamespace, r)
		var validationErr *services.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, `component "greeter" not found`)
	})

	t.Run("owning component in another project", func(t *testing.T) {
		svc := newService(t, testutil.NewComponent(testNamespace, "other-project", testComponent))
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		r.Spec.Owner.ComponentName = testComponent

		_, err := svc.CreateEndpoint(ctx, testNamespace, r)
		var validationErr *services.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, `belongs to project "other-project"`)
	})

	t.Run("namespace overrides body namespace", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint("ignored", testProject, "test-r")

		result, err := svc.CreateEndpoint(ctx, testNamespace, r)
		require.NoError(t, err)
		assert.Equal(t, testNamespace, result.Namespace)
	})
}

func TestUpdateEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, existing)

		update := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		update.Labels = map[string]string{"env": "prod"}

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "prod", result.Labels["env"])
		assert.Equal(t, testProject, result.Labels[labels.LabelKeyProjectName])
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateEndpoint(ctx, testNamespace, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot be nil")
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		r := testutil.NewEndpoint(testNamespace, testProject, "nonexistent")

		_, err := svc.UpdateEndpoint(ctx, testNamespace, r)
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("a backend on another component moves ownership to the project", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		existing.Spec.Owner.ComponentName = testComponent
		svc := newService(t, existing, testutil.NewComponent(testNamespace, testProject, testComponent))

		update := existing.DeepCopy()
		update.Spec.BackendRefs = append(update.Spec.BackendRefs, openchoreov1alpha1.EndpointBackendRef{
			Name: "canary", Kind: openchoreov1alpha1.EndpointBackendKindComponent,
			Component: &openchoreov1alpha1.EndpointComponentBackend{Name: "greeter-v2", Endpoint: "http"},
		})

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Empty(t, result.Spec.Owner.ComponentName)
		assert.Equal(t, testProject, result.Spec.Owner.ProjectName)
	})

	t.Run("keeps the owning component while every backend routes to it", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		existing.Spec.Owner.ComponentName = testComponent
		svc := newService(t, existing, testutil.NewComponent(testNamespace, testProject, testComponent))

		update := existing.DeepCopy()
		update.Spec.Visibility = []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityInternal}

		result, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, testComponent, result.Spec.Owner.ComponentName)
	})

	t.Run("project reassignment rejected", func(t *testing.T) {
		existing := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, existing)

		update := testutil.NewEndpoint(testNamespace, "different-project", "test-r")

		_, err := svc.UpdateEndpoint(ctx, testNamespace, update)
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
		assert.Contains(t, validationErr.Msg, "immutable")
	})
}

func TestListEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("success with items", func(t *testing.T) {
		r1 := testutil.NewEndpoint(testNamespace, testProject, "r-1")
		r2 := testutil.NewEndpoint(testNamespace, testProject, "r-2")
		svc := newService(t, r1, r2)

		result, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, endpointTypeMeta, item.TypeMeta)
		}
	})

	t.Run("filters by project", func(t *testing.T) {
		// Two projects in the same namespace
		other := testutil.NewProject(testNamespace, "other-project")
		r1 := testutil.NewEndpoint(testNamespace, testProject, "r-mine")
		r2 := testutil.NewEndpoint(testNamespace, "other-project", "r-other")
		svc := newService(t, other, r1, r2)

		result, err := svc.ListEndpoints(ctx, testNamespace, testProject, services.ListOptions{})
		require.NoError(t, err)
		require.Len(t, result.Items, 1)
		assert.Equal(t, "r-mine", result.Items[0].Name)
	})

	t.Run("project filter rejects missing project", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpoints(ctx, testNamespace, "missing", services.ListOptions{})
		require.ErrorIs(t, err, projectsvc.ErrProjectNotFound)
	})

	t.Run("empty list", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("invalid label selector", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEndpoints(ctx, testNamespace, "", services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})
}

func TestGetEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, r)

		result, err := svc.GetEndpoint(ctx, testNamespace, "test-r")
		require.NoError(t, err)
		assert.Equal(t, endpointTypeMeta, result.TypeMeta)
		assert.Equal(t, "test-r", result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEndpoint(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})
}

func TestDeleteEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		r := testutil.NewEndpoint(testNamespace, testProject, "test-r")
		svc := newService(t, r)

		err := svc.DeleteEndpoint(ctx, testNamespace, "test-r")
		require.NoError(t, err)

		_, err = svc.GetEndpoint(ctx, testNamespace, "test-r")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEndpoint(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEndpointNotFound)
	})
}
