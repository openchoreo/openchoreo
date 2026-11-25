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

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// newReconcilerWithEnvironmentIndex registers the spec.environment index the production
// controller sets up, so findReleaseBindingsForEnvironment can list by it.
func newReconcilerWithEnvironmentIndex(t *testing.T, objs ...client.Object) *Reconciler {
	t.Helper()
	c := fake.NewClientBuilder().
		WithScheme(watchTestScheme(t)).
		WithObjects(objs...).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{}, environmentIndex, indexEnvironment).
		Build()
	return &Reconciler{Client: c}
}

func TestIndexEnvironment(t *testing.T) {
	assert.Equal(t, []string{"qa"}, indexEnvironment(bindingFor("org", "rb", "qa")))
	assert.Nil(t, indexEnvironment(bindingFor("org", "rb", "")), "an unset environment must not be indexed")
}

// TestFindReleaseBindingsForEnvironment: an Environment event enqueues only the bindings in
// its namespace whose spec.environment names it.
func TestFindReleaseBindingsForEnvironment(t *testing.T) {
	ctx := context.Background()
	r := newReconcilerWithEnvironmentIndex(t,
		bindingFor("org", "rb-qa-1", "qa"),
		bindingFor("org", "rb-qa-2", "qa"),
		bindingFor("org", "rb-dev", "dev"),
		bindingFor("other", "rb-qa-other-ns", "qa"),
	)

	t.Run("matches_bindings_for_that_environment_in_its_namespace", func(t *testing.T) {
		reqs := r.findReleaseBindingsForEnvironment(ctx,
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "qa"}})
		assert.ElementsMatch(t, []string{"rb-qa-1", "rb-qa-2"}, requestNames(reqs))
		for _, req := range reqs {
			assert.Equal(t, "org", req.Namespace)
		}
	})

	t.Run("environment_without_bindings_yields_nothing", func(t *testing.T) {
		reqs := r.findReleaseBindingsForEnvironment(ctx,
			&openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "prod"}})
		assert.Empty(t, reqs)
	})

	t.Run("wrong_object_type_yields_nothing", func(t *testing.T) {
		reqs := r.findReleaseBindingsForEnvironment(ctx,
			&openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Namespace: "org", Name: "qa"}})
		assert.Empty(t, reqs)
	})
}
