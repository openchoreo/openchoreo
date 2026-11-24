// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

func componentReleaseObj(name, component string) *openchoreov1alpha1.ComponentRelease {
	return &openchoreov1alpha1.ComponentRelease{
		ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: name},
		Spec: openchoreov1alpha1.ComponentReleaseSpec{
			Owner: openchoreov1alpha1.ComponentReleaseOwner{ProjectName: "proj", ComponentName: component},
		},
	}
}

func ownedBinding(ns, name, component, releaseName string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: component},
			Environment: "development",
			ReleaseName: releaseName,
		},
	}
}

// newReconcilerWithOwnerIndex registers the shared owner-component index the production
// manager sets up in controller.SetupSharedIndexes, so MatchingFields lookups work.
func newReconcilerWithOwnerIndex(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(watchTestScheme(t)).
		WithObjects(objs...).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{}, controller.IndexKeyReleaseBindingOwnerComponentName,
			func(obj client.Object) []string {
				rb := obj.(*openchoreov1alpha1.ReleaseBinding)
				if rb.Spec.Owner.ComponentName == "" {
					return nil
				}
				return []string{rb.Spec.Owner.ComponentName}
			}).
		Build()
	return &Reconciler{Client: c}
}

// TestFindReleaseBindingsForComponentRelease: a ComponentRelease enqueues only the bindings of
// its own component (in its own namespace) that reference it by name, so a binding reconciled
// before the cache observed the release is re-driven once the release appears.
func TestFindReleaseBindingsForComponentRelease(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWithOwnerIndex(t,
		ownedBinding("org", "comp-a-development", "comp-a", "comp-a-111"),
		ownedBinding("org", "comp-a-staging", "comp-a", "comp-a-000"),       // pinned to an older release
		ownedBinding("org", "comp-b-development", "comp-b", "comp-a-111"),   // other component, same release name
		ownedBinding("other", "comp-a-development", "comp-a", "comp-a-111"), // other namespace
	)

	t.Run("matches_bindings_referencing_the_release", func(t *testing.T) {
		reqs := r.findReleaseBindingsForComponentRelease(ctx, componentReleaseObj("comp-a-111", "comp-a"))
		assert.ElementsMatch(t, []string{"comp-a-development"}, requestNames(reqs))
		for _, req := range reqs {
			assert.Equal(t, "org", req.Namespace)
		}
	})

	t.Run("release_no_binding_references_yields_nothing", func(t *testing.T) {
		reqs := r.findReleaseBindingsForComponentRelease(ctx, componentReleaseObj("comp-a-222", "comp-a"))
		assert.Empty(t, reqs)
	})

	t.Run("release_without_owner_component_yields_nothing", func(t *testing.T) {
		reqs := r.findReleaseBindingsForComponentRelease(ctx, componentReleaseObj("comp-a-111", ""))
		assert.Empty(t, reqs)
	})
}

// TestComponentReleaseCreatedPredicate: only creation re-drives bindings. The spec is
// immutable, so updates (including the controller's hash-restore writes) and deletes are ignored.
func TestComponentReleaseCreatedPredicate(t *testing.T) {
	p := componentReleaseCreatedPredicate()
	cr := componentReleaseObj("comp-a-111", "comp-a")

	assert.True(t, p.Create(event.CreateEvent{Object: cr}), "create should pass")
	assert.False(t, p.Update(event.UpdateEvent{ObjectOld: cr, ObjectNew: cr}), "update should be ignored")
	assert.False(t, p.Delete(event.DeleteEvent{Object: cr}), "delete should be ignored")
	assert.False(t, p.Generic(event.GenericEvent{Object: cr}), "generic should be ignored")
}
