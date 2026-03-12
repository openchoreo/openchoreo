// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services/testutil"
)

// --- Test helpers ---

func newService(t *testing.T, objs ...client.Object) Service {
	t.Helper()
	return NewService(testutil.NewFakeClient(objs...), testutil.TestLogger())
}

const (
	testNamespace = "test-ns"
	testEnvName   = "test-env"
)

func testEnvironment() *openchoreov1alpha1.Environment {
	env := testutil.NewEnvironment(testNamespace, testEnvName)
	env.Spec.DataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
		Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
		Name: controller.DefaultPlaneName,
	}
	return env
}

func testDefaultDataPlane() *openchoreov1alpha1.DataPlane {
	return testutil.NewDataPlane(testNamespace, controller.DefaultPlaneName)
}

// --- ListEnvironments ---

func TestListEnvironments(t *testing.T) {
	ctx := context.Background()

	t.Run("success - returns items", func(t *testing.T) {
		env1 := testEnvironment()
		env2 := testutil.NewEnvironment(testNamespace, "env-2")
		svc := newService(t, env1, env2)

		result, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 2)
		for _, item := range result.Items {
			assert.Equal(t, environmentTypeMeta, item.TypeMeta)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		svc := newService(t)

		result, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, result.Items)
	})

	t.Run("namespace isolation", func(t *testing.T) {
		envInNs := testEnvironment()
		envInOtherNs := testutil.NewEnvironment("other-ns", "env-other")
		svc := newService(t, envInNs, envInOtherNs)

		result, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, testEnvName, result.Items[0].Name)
	})

	t.Run("valid label selector filters results", func(t *testing.T) {
		envWithLabel := testutil.NewEnvironment(testNamespace, "env-prod")
		envWithLabel.Labels = map[string]string{"tier": "prod"}
		envWithoutLabel := testutil.NewEnvironment(testNamespace, "env-dev")
		svc := newService(t, envWithLabel, envWithoutLabel)

		result, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{LabelSelector: "tier=prod"})
		require.NoError(t, err)
		assert.Len(t, result.Items, 1)
		assert.Equal(t, "env-prod", result.Items[0].Name)
	})

	t.Run("invalid label selector returns error", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{LabelSelector: "===invalid"})
		require.Error(t, err)
		var validationErr *services.ValidationError
		assert.ErrorAs(t, err, &validationErr)
	})

	t.Run("with limit does not error", func(t *testing.T) {
		// The fake client does not enforce Limit, so we can only verify
		// that passing a limit does not cause an error. Actual limit-to-
		// ListOption translation is covered by TestBuildListOptions.
		// TODO: use fake client interceptors to assert Limit is forwarded.
		objs := make([]client.Object, 0, 3)
		for i := range 3 {
			objs = append(objs, testutil.NewEnvironment(testNamespace, "env-"+string(rune('a'+i))))
		}
		svc := newService(t, objs...)

		result, err := svc.ListEnvironments(ctx, testNamespace, services.ListOptions{Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, result.Items)
	})
}

// --- GetEnvironment ---

func TestGetEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("found", func(t *testing.T) {
		svc := newService(t, testEnvironment())

		result, err := svc.GetEnvironment(ctx, testNamespace, testEnvName)
		require.NoError(t, err)
		assert.Equal(t, environmentTypeMeta, result.TypeMeta)
		assert.Equal(t, testEnvName, result.Name)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.GetEnvironment(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})
}

// --- CreateEnvironment ---

func TestCreateEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t, testDefaultDataPlane())
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "new-env"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: controller.DefaultPlaneName,
				},
			},
		}

		result, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.NoError(t, err)
		assert.Equal(t, environmentTypeMeta, result.TypeMeta)
		assert.Equal(t, testNamespace, result.Namespace)
	})

	t.Run("already exists", func(t *testing.T) {
		existing := testEnvironment()
		svc := newService(t, existing)
		dup := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: testEnvName},
		}

		_, err := svc.CreateEnvironment(ctx, testNamespace, dup)
		require.ErrorIs(t, err, ErrEnvironmentAlreadyExists)
	})

	t.Run("default dataplane resolution", func(t *testing.T) {
		svc := newService(t, testDefaultDataPlane())
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "auto-dp-env"},
		}

		result, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.NoError(t, err)
		require.NotNil(t, result.Spec.DataPlaneRef)
		assert.Equal(t, openchoreov1alpha1.DataPlaneRefKindDataPlane, result.Spec.DataPlaneRef.Kind)
		assert.Equal(t, controller.DefaultPlaneName, result.Spec.DataPlaneRef.Name)
	})

	t.Run("default dataplane not found", func(t *testing.T) {
		svc := newService(t) // no DataPlane seeded
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "no-dp-env"},
		}

		_, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.ErrorIs(t, err, ErrDataPlaneNotFound)
	})

	t.Run("dataplane ref kind defaults to DataPlane", func(t *testing.T) {
		svc := newService(t, testDefaultDataPlane())
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "kind-default-env"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Name: controller.DefaultPlaneName,
					// Kind intentionally empty
				},
			},
		}

		result, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.NoError(t, err)
		assert.Equal(t, openchoreov1alpha1.DataPlaneRefKindDataPlane, result.Spec.DataPlaneRef.Kind)
	})

	t.Run("explicit DataPlaneRef preserved", func(t *testing.T) {
		customDP := testutil.NewDataPlane(testNamespace, "custom-dp")
		svc := newService(t, testDefaultDataPlane(), customDP)
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "explicit-dp-env"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: "custom-dp",
				},
			},
		}

		result, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.NoError(t, err)
		assert.Equal(t, "custom-dp", result.Spec.DataPlaneRef.Name)
	})

	t.Run("status cleared", func(t *testing.T) {
		svc := newService(t, testDefaultDataPlane())
		env := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "status-env"},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: controller.DefaultPlaneName,
				},
			},
			Status: openchoreov1alpha1.EnvironmentStatus{
				ObservedGeneration: 99,
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue},
				},
			},
		}

		result, err := svc.CreateEnvironment(ctx, testNamespace, env)
		require.NoError(t, err)
		assert.Equal(t, openchoreov1alpha1.EnvironmentStatus{}, result.Status)
	})
}

// --- UpdateEnvironment ---

func TestUpdateEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		existing := testEnvironment()
		svc := newService(t, existing)

		update := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: testEnvName},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				IsProduction: true,
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: controller.DefaultPlaneName,
				},
			},
		}

		result, err := svc.UpdateEnvironment(ctx, testNamespace, update)
		require.NoError(t, err)
		assert.Equal(t, environmentTypeMeta, result.TypeMeta)
		assert.True(t, result.Spec.IsProduction)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)
		update := &openchoreov1alpha1.Environment{
			ObjectMeta: metav1.ObjectMeta{Name: "nonexistent"},
		}

		_, err := svc.UpdateEnvironment(ctx, testNamespace, update)
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})

	t.Run("nil input", func(t *testing.T) {
		svc := newService(t)

		_, err := svc.UpdateEnvironment(ctx, testNamespace, nil)
		require.ErrorIs(t, err, ErrEnvironmentNil)
	})
}

// --- DeleteEnvironment ---

func TestDeleteEnvironment(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc := newService(t, testEnvironment())

		err := svc.DeleteEnvironment(ctx, testNamespace, testEnvName)
		require.NoError(t, err)

		_, err = svc.GetEnvironment(ctx, testNamespace, testEnvName)
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})

	t.Run("not found", func(t *testing.T) {
		svc := newService(t)

		err := svc.DeleteEnvironment(ctx, testNamespace, "nonexistent")
		require.ErrorIs(t, err, ErrEnvironmentNotFound)
	})
}
