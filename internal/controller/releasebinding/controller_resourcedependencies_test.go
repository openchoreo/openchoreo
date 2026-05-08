// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/controller/resourcereleasebinding"
)

func TestBuildResourceDependencyTargets(t *testing.T) {
	t.Run("returns_empty_for_no_deps", func(t *testing.T) {
		rb := newRBForResourceDeps("ns1", "proj1", "comp1", "dev")
		targets := buildResourceDependencyTargets(rb, nil)
		assert.Empty(t, targets)
	})

	t.Run("returns_one_target_per_dep", func(t *testing.T) {
		rb := newRBForResourceDeps("ns1", "proj1", "comp1", "dev")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{
			{Ref: "orders-db"},
			{Ref: "cache"},
		}

		got := buildResourceDependencyTargets(rb, deps)
		want := []openchoreov1alpha1.ResourceDependencyTarget{
			{Namespace: "ns1", Project: "proj1", ResourceName: "orders-db", Environment: "dev"},
			{Namespace: "ns1", Project: "proj1", ResourceName: "cache", Environment: "dev"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("targets mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("uses_consumers_namespace_project_environment", func(t *testing.T) {
		// v1.1 is project-bound only: target namespace + project + environment all
		// come from the consuming ReleaseBinding, not from the dep itself.
		rb := newRBForResourceDeps("alt-ns", "alt-proj", "comp1", "prod")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "shared-db"}}

		got := buildResourceDependencyTargets(rb, deps)
		want := []openchoreov1alpha1.ResourceDependencyTarget{
			{Namespace: "alt-ns", Project: "alt-proj", ResourceName: "shared-db", Environment: "prod"},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("target context mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("preserves_dep_declaration_order", func(t *testing.T) {
		rb := newRBForResourceDeps("ns1", "proj1", "comp1", "dev")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{
			{Ref: "z-last"},
			{Ref: "a-first"},
			{Ref: "m-middle"},
		}
		got := buildResourceDependencyTargets(rb, deps)
		// Targets must follow workload-spec declaration order, not be re-sorted.
		assert.Equal(t, "z-last", got[0].ResourceName)
		assert.Equal(t, "a-first", got[1].ResourceName)
		assert.Equal(t, "m-middle", got[2].ResourceName)
	})
}

func TestAllResourceDependenciesResolved(t *testing.T) {
	t.Run("returns_true_when_no_deps", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		assert.True(t, allResourceDependenciesResolved(rb, nil))
	})

	t.Run("returns_true_when_pending_list_empty", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "db"}}
		assert.True(t, allResourceDependenciesResolved(rb, deps))
	})

	t.Run("returns_false_when_any_dep_pending", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		rb.Status.PendingResourceDependencies = []openchoreov1alpha1.PendingResourceDependency{
			{Namespace: "ns", Project: "proj", ResourceName: "db", Reason: "BindingNotFound"},
		}
		deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "db"}}
		assert.False(t, allResourceDependenciesResolved(rb, deps))
	})
}

func TestSetResourceDependenciesCondition(t *testing.T) {
	t.Run("no_targets_marks_true_with_no_resource_dependencies_reason", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		setResourceDependenciesCondition(rb, true)

		cond := findCondition(rb.Status.Conditions, string(ConditionResourceDependenciesReady))
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(ReasonNoResourceDependencies), cond.Reason)
	})

	t.Run("all_resolved_marks_true_with_all_ready_reason", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		rb.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
			{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
		}
		setResourceDependenciesCondition(rb, true)

		cond := findCondition(rb.Status.Conditions, string(ConditionResourceDependenciesReady))
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionTrue, cond.Status)
		assert.Equal(t, string(ReasonAllResourceDependenciesReady), cond.Reason)
	})

	t.Run("pending_marks_false_with_pending_reason", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		rb.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
			{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
			{Namespace: "ns", Project: "proj", ResourceName: "cache", Environment: "dev"},
		}
		rb.Status.PendingResourceDependencies = []openchoreov1alpha1.PendingResourceDependency{
			{Namespace: "ns", Project: "proj", ResourceName: "db", Reason: "BindingNotFound"},
		}
		setResourceDependenciesCondition(rb, false)

		cond := findCondition(rb.Status.Conditions, string(ConditionResourceDependenciesReady))
		require.NotNil(t, cond)
		assert.Equal(t, metav1.ConditionFalse, cond.Status)
		assert.Equal(t, string(ReasonResourceDependenciesPending), cond.Reason)
	})

	t.Run("message_includes_pending_and_resolved_counts", func(t *testing.T) {
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		rb.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
			{Namespace: "ns", Project: "proj", ResourceName: "a", Environment: "dev"},
			{Namespace: "ns", Project: "proj", ResourceName: "b", Environment: "dev"},
			{Namespace: "ns", Project: "proj", ResourceName: "c", Environment: "dev"},
		}
		rb.Status.PendingResourceDependencies = []openchoreov1alpha1.PendingResourceDependency{
			{Namespace: "ns", Project: "proj", ResourceName: "a", Reason: "BindingNotFound"},
		}
		setResourceDependenciesCondition(rb, false)

		cond := findCondition(rb.Status.Conditions, string(ConditionResourceDependenciesReady))
		require.NotNil(t, cond)
		assert.Contains(t, cond.Message, "1")
		assert.Contains(t, cond.Message, "2")
	})
}

func TestMakeResourceReleaseBindingOwnerEnvKey(t *testing.T) {
	got := controller.MakeResourceReleaseBindingOwnerEnvKey("proj1", "orders-db", "prod")
	assert.Equal(t, "proj1/orders-db/prod", got)
}

// Locks the load-bearing invariant for the reverse-watch lookup: a target derived from a
// consumer ReleaseBinding's workload deps must produce the same key that the index extracts
// from a provider ResourceReleaseBinding for the same (project, resource, env) tuple. If a
// future refactor changes the separator on one side, this test breaks and the resolver
// lookup silently returns no provider.
func TestResourceDependencyTargetIndexKeyRoundTrip(t *testing.T) {
	rb := newRBForResourceDeps("ns1", "proj1", "comp1", "prod")
	deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "orders-db"}}
	target := buildResourceDependencyTargets(rb, deps)[0]

	consumerKey := controller.MakeResourceReleaseBindingOwnerEnvKey(
		target.Project, target.ResourceName, target.Environment,
	)

	provider := &openchoreov1alpha1.ResourceReleaseBinding{
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  "proj1",
				ResourceName: "orders-db",
			},
			Environment: "prod",
		},
	}
	indexKeys := controller.IndexResourceReleaseBindingOwnerEnv(provider)
	require.Len(t, indexKeys, 1)
	assert.Equal(t, consumerKey, indexKeys[0])
}

func TestResolveResourceDependency(t *testing.T) {
	t.Run("binding_not_found_returns_pending", func(t *testing.T) {
		r := newResourceDepReconciler(t)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{Ref: "orders-db"}

		item, pending, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.NoError(t, err)
		assert.Nil(t, item)
		require.NotNil(t, pending)
		assert.Equal(t, "orders-db", pending.ResourceName)
		assert.Contains(t, pending.Reason, "not found")
	})

	t.Run("multiple_bindings_found_returns_pending", func(t *testing.T) {
		// Two RRBs share the same (project, resource, env) — should never happen, but the
		// resolver must surface this defensively rather than picking arbitrarily.
		dup1 := newProviderRRB("ns", "proj", "orders-db", "dev", "rrb1", true, nil)
		dup2 := newProviderRRB("ns", "proj", "orders-db", "dev", "rrb2", true, nil)
		r := newResourceDepReconciler(t, dup1, dup2)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{Ref: "orders-db"}

		item, pending, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.NoError(t, err)
		assert.Nil(t, item)
		require.NotNil(t, pending)
		assert.Contains(t, pending.Reason, "multiple")
	})

	t.Run("provider_not_ready_returns_pending", func(t *testing.T) {
		rrb := newProviderRRB("ns", "proj", "orders-db", "dev", "rrb1", false, nil)
		r := newResourceDepReconciler(t, rrb)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{Ref: "orders-db"}

		item, pending, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.NoError(t, err)
		assert.Nil(t, item)
		require.NotNil(t, pending)
		assert.Contains(t, pending.Reason, "not ready")
	})

	t.Run("provider_ready_but_referenced_output_missing_returns_pending", func(t *testing.T) {
		// Provider is Ready but its outputs[] doesn't include the binding's referenced name.
		rrb := newProviderRRB("ns", "proj", "orders-db", "dev", "rrb1", true,
			[]openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: "10.0.0.5"},
			})
		r := newResourceDepReconciler(t, rrb)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"password": "DB_PASS"},
		}

		item, pending, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.NoError(t, err)
		assert.Nil(t, item)
		require.NotNil(t, pending)
		assert.Contains(t, pending.Reason, "password")
	})

	t.Run("provider_ready_with_outputs_returns_item", func(t *testing.T) {
		rrb := newProviderRRB("ns", "proj", "orders-db", "dev", "rrb1", true,
			[]openchoreov1alpha1.ResolvedResourceOutput{
				{Name: "host", Value: "10.0.0.5"},
			})
		r := newResourceDepReconciler(t, rrb)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{
			Ref:         "orders-db",
			EnvBindings: map[string]string{"host": "DB_HOST"},
		}

		item, pending, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.NoError(t, err)
		assert.Nil(t, pending)
		require.NotNil(t, item)
		assert.Equal(t, "orders-db", item.Ref)
		require.Len(t, item.EnvVars, 1)
		assert.Equal(t, "DB_HOST", item.EnvVars[0].Name)
		assert.Equal(t, "10.0.0.5", item.EnvVars[0].Value)
	})

	t.Run("transient_api_error_propagates", func(t *testing.T) {
		// Inject a list error to verify the resolver propagates it (caller requeues).
		listErr := errors.New("etcd unavailable")
		r := newResourceDepReconciler(t)
		r.Client = fake.NewClientBuilder().
			WithScheme(r.Scheme).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				controller.IndexKeyResourceReleaseBindingOwnerEnv,
				controller.IndexResourceReleaseBindingOwnerEnv).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*openchoreov1alpha1.ResourceReleaseBindingList); ok {
						return listErr
					}
					return c.List(ctx, list, opts...)
				},
			}).
			Build()
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		dep := openchoreov1alpha1.WorkloadResourceDependency{Ref: "orders-db"}

		_, _, err := r.resolveResourceDependency(context.Background(), rb, dep)
		require.Error(t, err)
		assert.ErrorIs(t, err, listErr)
	})
}

func TestResolveResourceDependencies(t *testing.T) {
	t.Run("empty_deps_returns_empty_lists", func(t *testing.T) {
		r := newResourceDepReconciler(t)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")

		items, pending, err := r.resolveResourceDependencies(context.Background(), rb, nil)
		require.NoError(t, err)
		assert.Empty(t, items)
		assert.Empty(t, pending)
	})

	t.Run("mixed_resolved_and_pending", func(t *testing.T) {
		// db is resolved, cache has no provider RRB.
		dbRRB := newProviderRRB("ns", "proj", "db", "dev", "db-binding", true,
			[]openchoreov1alpha1.ResolvedResourceOutput{{Name: "host", Value: "h"}})
		r := newResourceDepReconciler(t, dbRRB)
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{
			{Ref: "db", EnvBindings: map[string]string{"host": "DB_HOST"}},
			{Ref: "cache"},
		}

		items, pending, err := r.resolveResourceDependencies(context.Background(), rb, deps)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "db", items[0].Ref)
		require.Len(t, pending, 1)
		assert.Equal(t, "cache", pending[0].ResourceName)
	})

	t.Run("api_error_aborts_orchestrator", func(t *testing.T) {
		// One dep's lookup fails transiently → orchestrator returns error.
		listErr := errors.New("etcd down")
		scheme := runtime.NewScheme()
		require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
		c := fake.NewClientBuilder().
			WithScheme(scheme).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				controller.IndexKeyResourceReleaseBindingOwnerEnv,
				controller.IndexResourceReleaseBindingOwnerEnv).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*openchoreov1alpha1.ResourceReleaseBindingList); ok {
						return listErr
					}
					return c.List(ctx, list, opts...)
				},
			}).
			Build()
		r := &Reconciler{Client: c, Scheme: scheme}
		rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
		deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "db"}}

		_, _, err := r.resolveResourceDependencies(context.Background(), rb, deps)
		require.Error(t, err)
	})
}

// --- helpers ---

func newResourceDepReconciler(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
			controller.IndexKeyResourceReleaseBindingOwnerEnv,
			controller.IndexResourceReleaseBindingOwnerEnv)
	if len(objs) > 0 {
		builder = builder.WithObjects(objs...)
	}
	return &Reconciler{Client: builder.Build(), Scheme: scheme}
}

func newProviderRRB(namespace, project, resource, environment, name string, ready bool,
	outputs []openchoreov1alpha1.ResolvedResourceOutput) *openchoreov1alpha1.ResourceReleaseBinding {
	cond := metav1.Condition{
		Type:               string(resourcereleasebinding.ConditionReady),
		Status:             metav1.ConditionFalse,
		Reason:             "Pending",
		Message:            "not yet ready",
		LastTransitionTime: metav1.Now(),
	}
	if ready {
		cond.Status = metav1.ConditionTrue
		cond.Reason = "Ready"
		cond.Message = "ResourceReleaseBinding is ready"
	}
	return &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  project,
				ResourceName: resource,
			},
			Environment: environment,
		},
		Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
			Conditions: []metav1.Condition{cond},
			Outputs:    outputs,
		},
	}
}

func newRBForResourceDeps(namespace, project, component, environment string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      component + "-" + environment,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			Environment: environment,
		},
	}
}
