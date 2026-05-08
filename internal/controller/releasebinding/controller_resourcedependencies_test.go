// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
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

// --- helpers ---

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
