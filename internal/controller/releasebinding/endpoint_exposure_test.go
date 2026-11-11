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
	"github.com/openchoreo/openchoreo/internal/controller"
)

func exposureScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, openchoreov1alpha1.AddToScheme(scheme))
	return scheme
}

func exposingEndpoint(name, project string, visibility []openchoreov1alpha1.EndpointVisibility,
	backends ...openchoreov1alpha1.EndpointBackendRef) *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner:       openchoreov1alpha1.EndpointOwner{ProjectName: project},
			Visibility:  visibility,
			BackendRefs: backends,
		},
	}
}

func componentRef(component, endpoint string) openchoreov1alpha1.EndpointBackendRef {
	return openchoreov1alpha1.EndpointBackendRef{
		Name: component,
		Kind: openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{
			Name:     component,
			Endpoint: endpoint,
		},
	}
}

func TestEndpointVisibility(t *testing.T) {
	internal := openchoreov1alpha1.EndpointVisibilityInternal
	external := openchoreov1alpha1.EndpointVisibilityExternal
	namespace := openchoreov1alpha1.EndpointVisibilityNamespace

	objs := []client.Object{
		exposingEndpoint("orders-api", "store", []openchoreov1alpha1.EndpointVisibility{external, internal},
			componentRef("orders", "api")),
		// Another component's endpoint, and another project's Endpoint: ignored.
		exposingEndpoint("payments-api", "store", []openchoreov1alpha1.EndpointVisibility{external},
			componentRef("payments", "api")),
		exposingEndpoint("other-project", "billing", []openchoreov1alpha1.EndpointVisibility{namespace},
			componentRef("orders", "api")),
		// A workload endpoint the workload does not declare: ignored.
		exposingEndpoint("orders-missing", "store", []openchoreov1alpha1.EndpointVisibility{external},
			componentRef("orders", "missing")),
	}
	c := fake.NewClientBuilder().WithScheme(exposureScheme(t)).WithObjects(objs...).Build()
	r := &Reconciler{Client: c}

	endpoints := map[string]openchoreov1alpha1.WorkloadEndpoint{
		"api":     {Protocol: openchoreov1alpha1.EndpointProtocolHTTP, Port: 8080},
		"metrics": {Protocol: openchoreov1alpha1.EndpointProtocolHTTP, Port: 9090},
		"legacy": {
			Protocol:   openchoreov1alpha1.EndpointProtocolHTTP,
			Port:       8081,
			Visibility: []openchoreov1alpha1.EndpointVisibility{namespace}, //nolint:staticcheck // the deprecated field still counts
		},
	}

	got, err := r.endpointVisibility(context.Background(), "ns", "store", "orders", endpoints)
	require.NoError(t, err)
	assert.Equal(t, map[string][]openchoreov1alpha1.EndpointVisibility{
		"api":     {external, internal},
		"metrics": nil,
		"legacy":  {namespace},
	}, got)
}

func TestFindReleaseBindingsForEndpoint(t *testing.T) {
	binding := func(name, project, component string) *openchoreov1alpha1.ReleaseBinding {
		return &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns"},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{ProjectName: project, ComponentName: component},
			},
		}
	}
	c := fake.NewClientBuilder().
		WithScheme(exposureScheme(t)).
		WithObjects(
			binding("orders-dev", "store", "orders"),
			binding("orders-prod", "store", "orders"),
			binding("payments-dev", "store", "payments"),
			binding("other-orders-dev", "billing", "orders"),
		).
		WithIndex(&openchoreov1alpha1.ReleaseBinding{}, controller.IndexKeyReleaseBindingOwnerComponentName,
			func(obj client.Object) []string {
				return []string{obj.(*openchoreov1alpha1.ReleaseBinding).Spec.Owner.ComponentName}
			}).
		Build()
	r := &Reconciler{Client: c}

	ep := exposingEndpoint("orders-api", "store", nil,
		componentRef("orders", "api"), componentRef("orders", "admin"))
	requests := r.findReleaseBindingsForEndpoint(context.Background(), ep)
	names := make([]string, 0, len(requests))
	for _, req := range requests {
		names = append(names, req.Name)
	}
	assert.ElementsMatch(t, []string{"orders-dev", "orders-prod"}, names)
}
