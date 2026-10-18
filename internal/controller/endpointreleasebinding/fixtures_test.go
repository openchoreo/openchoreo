// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	testNamespace = "default"
	testProject   = "shop"
	testEnvName   = "dev"
	testEndpoint  = "greeter-api"
	testRelease   = "greeter-api-abc123"
)

// weightedRouteTemplate renders one HTTPRoute whose backendRefs carry every
// resolved backend's Service, port, and weight.
const weightedRouteTemplate = `{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "HTTPRoute",
  "metadata": {"name": "${metadata.name}", "namespace": "${metadata.namespace}", "labels": "${metadata.labels}"},
  "spec": {"rules": [{"backendRefs": "${backends.map(b, {\"name\": b.serviceName, \"port\": b.port, \"weight\": b.weight})}"}]}
}`

func newBinding(name, release string) *openchoreov1alpha1.EndpointReleaseBinding {
	return &openchoreov1alpha1.EndpointReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner: openchoreov1alpha1.EndpointReleaseBindingOwner{
				ProjectName:  testProject,
				EndpointName: testEndpoint,
			},
			Environment:     testEnvName,
			EndpointRelease: release,
		},
	}
}

func componentBackend(name, component string, weight int32) openchoreov1alpha1.EndpointBackendRef {
	return openchoreov1alpha1.EndpointBackendRef{
		Name:      name,
		Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
		Component: &openchoreov1alpha1.EndpointComponentBackend{Name: component, Endpoint: "http"},
		Weight:    ptr.To(weight),
	}
}

func newRelease(name string, backends ...openchoreov1alpha1.EndpointBackendRef) *openchoreov1alpha1.EndpointRelease {
	return &openchoreov1alpha1.EndpointRelease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace},
		Spec: openchoreov1alpha1.EndpointReleaseSpec{
			Owner: openchoreov1alpha1.EndpointReleaseOwner{ProjectName: testProject, EndpointName: testEndpoint},
			EndpointType: openchoreov1alpha1.EndpointReleaseEndpointType{
				Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
				Name: "weighted-route",
				Spec: openchoreov1alpha1.EndpointTypeSpec{
					Resources: []openchoreov1alpha1.ResourceTemplate{{
						ID:       "route",
						Template: &runtime.RawExtension{Raw: []byte(weightedRouteTemplate)},
					}},
				},
			},
			Visibility:  []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: backends,
		},
	}
}

func newEnvironment() *openchoreov1alpha1.Environment {
	return &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: testEnvName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
				Name: "primary",
			},
		},
	}
}

func newDataPlane() *openchoreov1alpha1.DataPlane {
	return &openchoreov1alpha1.DataPlane{ObjectMeta: metav1.ObjectMeta{Name: "primary", Namespace: testNamespace}}
}

func newProject() *openchoreov1alpha1.Project {
	return &openchoreov1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{Name: testProject, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type:                  openchoreov1alpha1.ProjectTypeRef{Name: "default"},
		},
	}
}

func newEndpointCR() *openchoreov1alpha1.Endpoint {
	return &openchoreov1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{Name: testEndpoint, Namespace: testNamespace},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner:       openchoreov1alpha1.EndpointOwner{ProjectName: testProject},
			Type:        openchoreov1alpha1.EndpointTypeRef{Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType, Name: "weighted-route"},
			Visibility:  []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: []openchoreov1alpha1.EndpointBackendRef{componentBackend("stable", "greeter", 1)},
		},
	}
}

// deployedComponent returns a ReleaseBinding reporting an in-cluster address
// for the component's "http" endpoint in serviceNamespace.
func deployedComponent(component, serviceNamespace string) *openchoreov1alpha1.ReleaseBinding {
	return &openchoreov1alpha1.ReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{Name: component + "-" + testEnvName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: testProject, ComponentName: component},
			Environment: testEnvName,
		},
		Status: openchoreov1alpha1.ReleaseBindingStatus{
			Endpoints: []openchoreov1alpha1.EndpointURLStatus{{
				Name: "http",
				Type: openchoreov1alpha1.EndpointProtocolHTTP,
				ServiceURL: &openchoreov1alpha1.EndpointURL{
					Scheme: "http",
					Host:   fmt.Sprintf("%s-svc.%s.svc.cluster.local", component, serviceNamespace),
					Port:   8080,
				},
			}},
		},
	}
}
