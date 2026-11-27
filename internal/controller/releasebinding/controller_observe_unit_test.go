// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

func observePRB(mode openchoreov1alpha1.BindingMode, namespace string) *openchoreov1alpha1.ProjectReleaseBinding {
	return &openchoreov1alpha1.ProjectReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-prod", Namespace: "acme"},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: "payments"},
			Environment: "prod",
			Mode:        mode,
		},
		Status: openchoreov1alpha1.ProjectReleaseBindingStatus{Namespace: namespace},
	}
}

func TestFindReleaseBindingsForProjectReleaseBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))

	binding := func(name, project, env string, mode openchoreov1alpha1.BindingMode) client.Object {
		return &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "acme"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: name},
				Environment: env,
				Mode:        mode,
			},
		}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		binding("api-prod", "payments", "prod", openchoreov1alpha1.BindingModeObserve),
		binding("worker-prod", "payments", "prod", openchoreov1alpha1.BindingModeManage),
		binding("api-dev", "payments", "dev", openchoreov1alpha1.BindingModeObserve),
		binding("orders-prod", "orders", "prod", openchoreov1alpha1.BindingModeObserve),
	).Build()
	r := &Reconciler{Client: c}

	got := r.findReleaseBindingsForProjectReleaseBinding(context.Background(), observePRB(openchoreov1alpha1.BindingModeObserve, "payments"))

	names := make([]string, 0, len(got))
	for _, req := range got {
		names = append(names, req.Name)
	}
	assert.ElementsMatch(t, []string{"api-prod", "worker-prod"}, names)
}
