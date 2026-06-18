// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
)

// prbTestScheme builds a scheme registered with the OpenChoreo API types. These
// fake-client unit tests run alongside the envtest Ginkgo suite, so they must
// not rely on the suite's BeforeSuite having registered the global scheme.
func prbTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("add openchoreo scheme: %v", err)
	}
	return s
}

func newPRBForEnv(name, env string, deleting bool) *openchoreov1alpha1.ProjectReleaseBinding {
	prb := &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "p"},
			Environment: env,
		},
	}
	if deleting {
		now := metav1.Now()
		prb.DeletionTimestamp = &now
		prb.Finalizers = []string{"openchoreo.dev/projectreleasebinding-cleanup"}
	}
	return prb
}

func TestDeleteAndCountProjectReleaseBindings(t *testing.T) {
	s := prbTestScheme(t)
	env := &openchoreov1alpha1.Environment{ObjectMeta: metav1.ObjectMeta{Name: "dev", Namespace: "ns"}}

	t.Run("counts matching bindings and leaves other environments untouched", func(t *testing.T) {
		match1 := newPRBForEnv("m1", "dev", false)
		match2 := newPRBForEnv("m2", "dev", false)
		other := newPRBForEnv("other", "prod", false)
		cli := fake.NewClientBuilder().WithScheme(s).WithObjects(match1, match2, other).Build()
		r := &Reconciler{Client: cli, Scheme: s}

		count, err := r.deleteAndCountProjectReleaseBindings(context.Background(), env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected count 2, got %d", count)
		}

		got := &openchoreov1alpha1.ProjectReleaseBinding{}
		if err := cli.Get(context.Background(), client.ObjectKey{Name: "other", Namespace: "ns"}, got); err != nil {
			t.Fatalf("other-environment binding should remain: %v", err)
		}
		if got.DeletionTimestamp != nil {
			t.Fatal("other-environment binding should not be marked for deletion")
		}
	})

	t.Run("counts an already-deleting binding without re-deleting it", func(t *testing.T) {
		deleting := newPRBForEnv("d1", "dev", true)
		cli := fake.NewClientBuilder().WithScheme(s).WithObjects(deleting).Build()
		r := &Reconciler{Client: cli, Scheme: s}

		count, err := r.deleteAndCountProjectReleaseBindings(context.Background(), env)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Fatalf("expected count 1, got %d", count)
		}
	})

	t.Run("returns an error when listing bindings fails", func(t *testing.T) {
		cli := fake.NewClientBuilder().WithScheme(s).
			WithInterceptorFuncs(interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*openchoreov1alpha1.ProjectReleaseBindingList); ok {
						return errors.New("simulated list error")
					}
					return c.List(ctx, list, opts...)
				},
			}).Build()
		r := &Reconciler{Client: cli, Scheme: s}

		if _, err := r.deleteAndCountProjectReleaseBindings(context.Background(), env); err == nil {
			t.Fatal("expected error when listing project release bindings fails")
		}
	})

	t.Run("returns an error when deleting a binding fails", func(t *testing.T) {
		match := newPRBForEnv("m1", "dev", false)
		cli := fake.NewClientBuilder().WithScheme(s).WithObjects(match).
			WithInterceptorFuncs(interceptor.Funcs{
				Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
					if _, ok := obj.(*openchoreov1alpha1.ProjectReleaseBinding); ok {
						return errors.New("simulated delete error")
					}
					return c.Delete(ctx, obj, opts...)
				},
			}).Build()
		r := &Reconciler{Client: cli, Scheme: s}

		if _, err := r.deleteAndCountProjectReleaseBindings(context.Background(), env); err == nil {
			t.Fatal("expected error when deleting a project release binding fails")
		}
	})
}

// TestFinalizePropagatesProjectReleaseBindingListError verifies that a failure
// listing ProjectReleaseBindings during environment finalization surfaces as an
// error (so the reconcile requeues) rather than letting the environment proceed
// to removal while bindings may still exist.
func TestFinalizePropagatesProjectReleaseBindingListError(t *testing.T) {
	s := prbTestScheme(t)
	env := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "dev",
			Namespace:  "ns",
			Finalizers: []string{EnvCleanupFinalizer},
		},
	}
	cli := fake.NewClientBuilder().WithScheme(s).
		WithObjects(env).
		WithStatusSubresource(&openchoreov1alpha1.Environment{}).
		WithIndex(&openchoreov1alpha1.DeploymentPipeline{}, controller.IndexKeyDeploymentPipelineEnvironmentRef,
			func(client.Object) []string { return nil }).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*openchoreov1alpha1.ProjectReleaseBindingList); ok {
					return errors.New("simulated list error")
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
	r := &Reconciler{Client: cli, Scheme: s}

	ctx := context.Background()
	if err := cli.Delete(ctx, env); err != nil {
		t.Fatalf("delete env: %v", err)
	}
	live := &openchoreov1alpha1.Environment{}
	if err := cli.Get(ctx, client.ObjectKey{Name: "dev", Namespace: "ns"}, live); err != nil {
		t.Fatalf("get env: %v", err)
	}

	// First finalize sets the Finalizing condition and returns; the second
	// proceeds to the binding-deletion step, where the ProjectReleaseBinding
	// list fails and the error must propagate.
	if _, err := r.finalize(ctx, live.DeepCopy(), live); err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	if _, err := r.finalize(ctx, live.DeepCopy(), live); err == nil {
		t.Fatal("expected error from ProjectReleaseBinding list failure during finalize")
	}
}
