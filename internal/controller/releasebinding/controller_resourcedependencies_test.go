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
	"sigs.k8s.io/controller-runtime/pkg/event"

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
		// Resource dependencies are project-bound: target namespace + project +
		// environment all come from the consuming ReleaseBinding, not from the dep itself.
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

func TestIsResourceReleaseBindingReady(t *testing.T) {
	makeRRB := func(generation int64, ready metav1.ConditionStatus, observedGen int64) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Generation: generation},
			Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
				Conditions: []metav1.Condition{{
					Type:               string(resourcereleasebinding.ConditionReady),
					Status:             ready,
					ObservedGeneration: observedGen,
					LastTransitionTime: metav1.Now(),
				}},
			},
		}
	}

	t.Run("ready_for_current_generation", func(t *testing.T) {
		assert.True(t, isResourceReleaseBindingReady(makeRRB(2, metav1.ConditionTrue, 2)))
	})
	t.Run("ready_for_stale_generation", func(t *testing.T) {
		// Provider mid-reconcile: spec advanced to gen 3, status still reflects gen 2.
		assert.False(t, isResourceReleaseBindingReady(makeRRB(3, metav1.ConditionTrue, 2)))
	})
	t.Run("not_ready", func(t *testing.T) {
		assert.False(t, isResourceReleaseBindingReady(makeRRB(2, metav1.ConditionFalse, 2)))
	})
	t.Run("missing_condition", func(t *testing.T) {
		rrb := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		assert.False(t, isResourceReleaseBindingReady(rrb))
	})
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
		dup1 := newProviderRRB("orders-db", "rrb1", true, nil)
		dup2 := newProviderRRB("orders-db", "rrb2", true, nil)
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
		rrb := newProviderRRB("orders-db", "rrb1", false, nil)
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
		rrb := newProviderRRB("orders-db", "rrb1", true,
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
		rrb := newProviderRRB("orders-db", "rrb1", true,
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
		dbRRB := newProviderRRB("db", "db-binding", true,
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

func TestMakeResourceDependencyTargetKey(t *testing.T) {
	got := makeResourceDependencyTargetKey("ns1", "proj1", "orders-db", "prod")
	assert.Equal(t, "ns1/proj1/orders-db/prod", got)
}

func TestSetupResourceDependencyTargetsIndex_indexerEmitsOneKeyPerTarget(t *testing.T) {
	// Build a ReleaseBinding with two distinct resource-dep targets and verify the
	// indexer emits both composite keys. The fake client's index is what production
	// uses to reverse-lookup consumers from a provider RRB event.
	rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
	rb.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
		{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
		{Namespace: "ns", Project: "proj", ResourceName: "cache", Environment: "dev"},
	}
	keys := indexResourceDependencyTargets(rb)
	assert.ElementsMatch(t, []string{
		"ns/proj/db/dev",
		"ns/proj/cache/dev",
	}, keys)
}

func TestSetupResourceDependencyTargetsIndex_dedupesIdenticalTargets(t *testing.T) {
	rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
	rb.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
		{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
		{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
	}
	keys := indexResourceDependencyTargets(rb)
	assert.Len(t, keys, 1)
}

func TestSetupResourceDependencyTargetsIndex_returnsNilForEmpty(t *testing.T) {
	rb := newRBForResourceDeps("ns", "proj", "comp", "dev")
	keys := indexResourceDependencyTargets(rb)
	assert.Nil(t, keys)
}

func TestFindConsumerReleaseBindingsForResourceReleaseBinding(t *testing.T) {
	// One consumer RB with a target pointing at provider (proj, db, dev) — should be enqueued.
	consumer := newRBForResourceDeps("ns", "proj", "consumer", "dev")
	consumer.Name = "consumer-rb"
	consumer.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
		{Namespace: "ns", Project: "proj", ResourceName: "db", Environment: "dev"},
	}
	// Another RB whose target points at a different resource — should NOT be enqueued.
	unrelated := newRBForResourceDeps("ns", "proj", "other", "dev")
	unrelated.Name = "other-rb"
	unrelated.Status.ResourceDependencyTargets = []openchoreov1alpha1.ResourceDependencyTarget{
		{Namespace: "ns", Project: "proj", ResourceName: "cache", Environment: "dev"},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(consumer, unrelated).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{},
			resourceDependencyTargetsIndex, func(obj client.Object) []string {
				return indexResourceDependencyTargets(obj.(*openchoreov1alpha1.ReleaseBinding))
			}).
		Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	provider := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "db-binding", Namespace: "ns"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName: "proj", ResourceName: "db",
			},
			Environment: "dev",
		},
	}
	got := r.findConsumerReleaseBindingsForResourceReleaseBinding(context.Background(), provider)
	require.Len(t, got, 1, "expected one consumer enqueued")
	assert.Equal(t, "consumer-rb", got[0].Name)
}

func TestFindConsumerReleaseBindingsForResourceReleaseBinding_noConsumers(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{},
			resourceDependencyTargetsIndex, func(obj client.Object) []string {
				return indexResourceDependencyTargets(obj.(*openchoreov1alpha1.ReleaseBinding))
			}).
		Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	provider := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "ns"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p", ResourceName: "r"},
			Environment: "e",
		},
	}
	got := r.findConsumerReleaseBindingsForResourceReleaseBinding(context.Background(), provider)
	assert.Empty(t, got)
}

func TestFindConsumerReleaseBindingsForResourceReleaseBinding_wrongType(t *testing.T) {
	r := newResourceDepReconciler(t)
	got := r.findConsumerReleaseBindingsForResourceReleaseBinding(context.Background(),
		&openchoreov1alpha1.ReleaseBinding{}) // wrong type
	assert.Nil(t, got)
}

func TestFindConsumerReleaseBindingsForResourceReleaseBinding_skipsMalformedRRB(t *testing.T) {
	// Defensive: an RRB with empty Owner fields should produce no matches rather than
	// listing every consumer that happens to carry a zero-value target.
	r := newResourceDepReconciler(t)
	cases := []struct {
		name string
		rrb  *openchoreov1alpha1.ResourceReleaseBinding
	}{
		{"empty_project", &openchoreov1alpha1.ResourceReleaseBinding{
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ResourceName: "db"},
				Environment: "dev",
			},
		}},
		{"empty_resource", &openchoreov1alpha1.ResourceReleaseBinding{
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p"},
				Environment: "dev",
			},
		}},
		{"empty_environment", &openchoreov1alpha1.ResourceReleaseBinding{
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{ProjectName: "p", ResourceName: "db"},
			},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.findConsumerReleaseBindingsForResourceReleaseBinding(context.Background(), c.rrb)
			assert.Nil(t, got)
		})
	}
}

// Locks the load-bearing invariant for the reverse-watch: a target produced from a
// consumer's workload by buildResourceDependencyTargets must round-trip through
// indexResourceDependencyTargets and findConsumerReleaseBindingsForResourceReleaseBinding's
// key construction to enqueue the same consumer. Catches any future divergence in the key
// shape that hand-constructed two-side tests would miss.
func TestReverseWatchKeyRoundTrip(t *testing.T) {
	consumer := newRBForResourceDeps("ns", "proj", "consumer", "prod")
	consumer.Name = "consumer-rb"
	deps := []openchoreov1alpha1.WorkloadResourceDependency{{Ref: "orders-db"}}
	consumer.Status.ResourceDependencyTargets = buildResourceDependencyTargets(consumer, deps)

	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(consumer).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{},
			resourceDependencyTargetsIndex, func(obj client.Object) []string {
				return indexResourceDependencyTargets(obj.(*openchoreov1alpha1.ReleaseBinding))
			}).
		Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	provider := &openchoreov1alpha1.ResourceReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-db-rrb", Namespace: "ns"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName: "proj", ResourceName: "orders-db",
			},
			Environment: "prod",
		},
	}
	got := r.findConsumerReleaseBindingsForResourceReleaseBinding(context.Background(), provider)
	require.Len(t, got, 1, "consumer must round-trip via the index when both sides agree on the key shape")
	assert.Equal(t, "consumer-rb", got[0].Name)
}

func TestResourceReleaseBindingOutputsChangedPredicate(t *testing.T) {
	pred := resourceReleaseBindingOutputsChangedPredicate()

	t.Run("fires_on_create", func(t *testing.T) {
		assert.True(t, pred.Create(event.CreateEvent{Object: &openchoreov1alpha1.ResourceReleaseBinding{}}))
	})

	t.Run("fires_on_delete", func(t *testing.T) {
		assert.True(t, pred.Delete(event.DeleteEvent{Object: &openchoreov1alpha1.ResourceReleaseBinding{}}))
	})

	t.Run("does_not_fire_on_generic_event", func(t *testing.T) {
		assert.False(t, pred.Generic(event.GenericEvent{Object: &openchoreov1alpha1.ResourceReleaseBinding{}}))
	})

	t.Run("fires_on_outputs_change", func(t *testing.T) {
		old := &openchoreov1alpha1.ResourceReleaseBinding{
			Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
				Outputs: []openchoreov1alpha1.ResolvedResourceOutput{{Name: "host", Value: "1.1.1.1"}},
			},
		}
		new := old.DeepCopy()
		new.Status.Outputs[0].Value = "2.2.2.2"
		assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("fires_on_ready_condition_flip", func(t *testing.T) {
		old := &openchoreov1alpha1.ResourceReleaseBinding{
			Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Pending", LastTransitionTime: metav1.Now()},
				},
			},
		}
		new := old.DeepCopy()
		new.Status.Conditions[0].Status = metav1.ConditionTrue
		new.Status.Conditions[0].Reason = "Ready"
		assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("fires_on_generation_change", func(t *testing.T) {
		// PE edits the provider's spec → Generation advances. Even if the status hasn't
		// caught up, consumers must re-evaluate (the new check in
		// isResourceReleaseBindingReady will gate them off until ObservedGeneration matches).
		old := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Generation: 1},
		}
		new := old.DeepCopy()
		new.Generation = 2
		assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("fires_on_ready_observed_generation_change", func(t *testing.T) {
		// Provider catches up to a new generation: Ready=True stays, but ObservedGeneration
		// advances. Consumers were gated off (stale OG) and must now re-evaluate.
		old := &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Generation: 2},
			Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, ObservedGeneration: 1, Reason: "Ready", LastTransitionTime: metav1.Now()},
				},
			},
		}
		new := old.DeepCopy()
		new.Status.Conditions[0].ObservedGeneration = 2
		assert.True(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("does_not_fire_on_unrelated_status_change", func(t *testing.T) {
		// Same outputs, same Ready condition status. Some other condition (e.g., Synced)
		// changed — predicate should NOT fire because consumers don't care about that.
		old := &openchoreov1alpha1.ResourceReleaseBinding{
			Status: openchoreov1alpha1.ResourceReleaseBindingStatus{
				Outputs: []openchoreov1alpha1.ResolvedResourceOutput{{Name: "host", Value: "1.1.1.1"}},
				Conditions: []metav1.Condition{
					{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Ready", LastTransitionTime: metav1.Now()},
					{Type: "Synced", Status: metav1.ConditionTrue, Reason: "ReleaseSynced", LastTransitionTime: metav1.Now()},
				},
			},
		}
		new := old.DeepCopy()
		new.Status.Conditions[1].Reason = "ReleaseUpdated" // different non-Ready reason
		assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("does_not_fire_when_ready_absent_on_both_sides", func(t *testing.T) {
		old := &openchoreov1alpha1.ResourceReleaseBinding{}
		new := old.DeepCopy()
		assert.False(t, pred.Update(event.UpdateEvent{ObjectOld: old, ObjectNew: new}))
	})

	t.Run("rejects_wrong_type", func(t *testing.T) {
		// Defensive: predicate must not panic if an unexpected object kind arrives.
		assert.False(t, pred.Update(event.UpdateEvent{
			ObjectOld: &openchoreov1alpha1.ReleaseBinding{},
			ObjectNew: &openchoreov1alpha1.ReleaseBinding{},
		}))
	})
}

func TestSetReadyConditionWithResourceDependencies(t *testing.T) {
	t.Run("resource_deps_false_blocks_ready_with_its_reason", func(t *testing.T) {
		r := newTestReconciler()
		rb := makeReleaseBindingForConditions()
		setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
		setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")
		setConditionOnRB(rb, string(ConditionResourceDependenciesReady), metav1.ConditionFalse,
			string(ReasonResourceDependenciesPending), "1 dep pending")

		r.setReadyCondition(rb)

		ready := findCondition(rb.Status.Conditions, string(ConditionReady))
		require.NotNil(t, ready)
		assert.Equal(t, metav1.ConditionFalse, ready.Status)
		assert.Equal(t, string(ReasonResourceDependenciesPending), ready.Reason)
	})

	t.Run("resource_deps_true_with_others_true_yields_ready", func(t *testing.T) {
		r := newTestReconciler()
		rb := makeReleaseBindingForConditions()
		setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
		setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")
		setConditionOnRB(rb, string(ConditionResourceDependenciesReady), metav1.ConditionTrue,
			string(ReasonAllResourceDependenciesReady), "all resolved")

		r.setReadyCondition(rb)

		ready := findCondition(rb.Status.Conditions, string(ConditionReady))
		require.NotNil(t, ready)
		assert.Equal(t, metav1.ConditionTrue, ready.Status)
		assert.Equal(t, string(ReasonReady), ready.Reason)
	})

	t.Run("resource_deps_absent_does_not_block_ready", func(t *testing.T) {
		// Backward compat: a workload with no resource dependencies has no
		// ResourceDependenciesReady condition. Aggregate Ready must not require it.
		// Mirrors how ConnectionsResolved is treated as optional.
		r := newTestReconciler()
		rb := makeReleaseBindingForConditions()
		setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
		setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")
		// no ResourceDependenciesReady condition

		r.setReadyCondition(rb)

		ready := findCondition(rb.Status.Conditions, string(ConditionReady))
		require.NotNil(t, ready)
		assert.Equal(t, metav1.ConditionTrue, ready.Status)
		assert.Equal(t, string(ReasonReady), ready.Reason)
	})

	t.Run("connections_false_takes_priority_over_resource_deps_false", func(t *testing.T) {
		// Locks the priority: ConnectionsResolved is reported above ResourceDependenciesReady
		// when both fail.
		r := newTestReconciler()
		rb := makeReleaseBindingForConditions()
		setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
		setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionTrue, string(ReasonReady), "ready")
		setConditionOnRB(rb, string(ConditionConnectionsResolved), metav1.ConditionFalse,
			string(ReasonConnectionsPending), "endpoint pending")
		setConditionOnRB(rb, string(ConditionResourceDependenciesReady), metav1.ConditionFalse,
			string(ReasonResourceDependenciesPending), "resource dep pending")

		r.setReadyCondition(rb)

		ready := findCondition(rb.Status.Conditions, string(ConditionReady))
		require.NotNil(t, ready)
		assert.Equal(t, metav1.ConditionFalse, ready.Status)
		assert.Equal(t, string(ReasonConnectionsPending), ready.Reason)
	})

	t.Run("resource_deps_false_takes_priority_over_resources_ready_false", func(t *testing.T) {
		// Locks the priority: ResourceDependenciesReady is reported above ResourcesReady
		// when both fail. Without this test, swapping the two priority branches in
		// setReadyCondition would still pass all other tests.
		r := newTestReconciler()
		rb := makeReleaseBindingForConditions()
		setConditionOnRB(rb, string(ConditionReleaseSynced), metav1.ConditionTrue, string(ReasonReleaseSynced), "synced")
		setConditionOnRB(rb, string(ConditionResourcesReady), metav1.ConditionFalse,
			string(ReasonResourcesDegraded), "primary degraded")
		setConditionOnRB(rb, string(ConditionResourceDependenciesReady), metav1.ConditionFalse,
			string(ReasonResourceDependenciesPending), "resource dep pending")

		r.setReadyCondition(rb)

		ready := findCondition(rb.Status.Conditions, string(ConditionReady))
		require.NotNil(t, ready)
		assert.Equal(t, metav1.ConditionFalse, ready.Status)
		assert.Equal(t, string(ReasonResourceDependenciesPending), ready.Reason)
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

// newProviderRRB builds a fixture in the canonical (ns, proj, dev) namespace/project/env
// triple that the resolver tests share. Vary `resource` and `name` per test case.
func newProviderRRB(resource, name string, ready bool,
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
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
			Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
				ProjectName:  "proj",
				ResourceName: resource,
			},
			Environment: "dev",
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
