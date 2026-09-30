// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

// Deleting a ReleaseBinding must also delete its hook WorkflowRuns right away.
// Garbage collection alone can leave a pending approval run around after the
// binding is gone, and an operator could still approve a deployment that no
// longer exists.

func finalizeRun(name, rb, purpose string) *openchoreov1alpha1.WorkflowRun {
	return &openchoreov1alpha1.WorkflowRun{
		ObjectMeta: metav1.ObjectMeta{Namespace: unitNS, Name: name, Labels: map[string]string{
			labels.LabelKeyWorkflowPurpose: purpose,
			labels.LabelKeyReleaseBinding:  rb,
		}},
	}
}

// newFinalizeHarness returns a reconciler over a fake client holding a
// terminating binding that is already marked Finalizing, so finalize goes
// straight to cleanup.
func newFinalizeHarness(t *testing.T, funcs *interceptor.Funcs, extra ...client.Object) (*Reconciler, client.Client, *openchoreov1alpha1.ReleaseBinding) {
	t.Helper()
	s := watchTestScheme(t)
	now := metav1.Now()
	rb := &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: unitNS, Name: unitRB, UID: "rb-uid", Generation: 1,
			DeletionTimestamp: &now, Finalizers: []string{ReleaseBindingFinalizer},
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: "proj", ComponentName: "comp"},
			Environment: unitEnv,
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Conditions: []metav1.Condition{NewReleaseBindingFinalizingCondition(1)},
		},
	}
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(append([]client.Object{rb}, extra...)...)
	if funcs != nil {
		b = b.WithInterceptorFuncs(*funcs)
	}
	c := b.Build()
	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(rb), stored))
	return &Reconciler{Client: c, Scheme: s}, c, stored
}

func runNames(t *testing.T, c client.Client) []string {
	t.Helper()
	list := &openchoreov1alpha1.WorkflowRunList{}
	require.NoError(t, c.List(context.Background(), list, client.InNamespace(unitNS)))
	names := make([]string, 0, len(list.Items))
	for _, r := range list.Items {
		names = append(names, r.Name)
	}
	return names
}

func deleteAllOfReturns(err error) *interceptor.Funcs {
	return &interceptor.Funcs{
		DeleteAllOf: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteAllOfOption) error {
			return err
		},
	}
}

// Only this binding's hook runs may be deleted: another binding's hook runs and
// this binding's ordinary (non-hook) runs are someone else's work.
func TestFinalize_DeletesOnlyThisBindingsHookRuns(t *testing.T) {
	r, c, rb := newFinalizeHarness(t, nil,
		finalizeRun("mine", unitRB, labels.LabelValueWorkflowPurposeDeploymentHook),
		finalizeRun("other-binding", "rb-2", labels.LabelValueWorkflowPurposeDeploymentHook),
		finalizeRun("not-a-hook", unitRB, "build"),
	)

	res, err := r.finalize(context.Background(), rb.DeepCopy(), rb)
	require.NoError(t, err)
	assert.Zero(t, res.RequeueAfter)

	assert.ElementsMatch(t, []string{"other-binding", "not-a-hook"}, runNames(t, c))
	// Finalizer removed, so the fake client completes the deletion.
	err = c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: unitRB}, &openchoreov1alpha1.ReleaseBinding{})
	assert.True(t, apierrors.IsNotFound(err), "binding should be gone, got %v", err)
}

// NotFound means nothing is left to clean up, so it must not block removing
// the finalizer.
func TestFinalize_HookRunDeleteNotFoundIsTolerated(t *testing.T) {
	notFound := apierrors.NewNotFound(schema.GroupResource{Group: "openchoreo.dev", Resource: "workflowruns"}, "")
	r, c, rb := newFinalizeHarness(t, deleteAllOfReturns(notFound))

	_, err := r.finalize(context.Background(), rb.DeepCopy(), rb)
	require.NoError(t, err)
	err = c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: unitRB}, &openchoreov1alpha1.ReleaseBinding{})
	assert.True(t, apierrors.IsNotFound(err), "binding should be gone, got %v", err)
}

// Any other delete error must be returned and the finalizer kept. If the
// binding were released anyway, a pending approval run could outlive it.
func TestFinalize_HookRunDeleteErrorKeepsFinalizer(t *testing.T) {
	r, c, rb := newFinalizeHarness(t, deleteAllOfReturns(errBoom))

	_, err := r.finalize(context.Background(), rb.DeepCopy(), rb)
	require.ErrorIs(t, err, errBoom)

	stored := &openchoreov1alpha1.ReleaseBinding{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: unitNS, Name: unitRB}, stored))
	assert.Contains(t, stored.Finalizers, ReleaseBindingFinalizer)
}
