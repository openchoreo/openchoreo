// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointreleasebinding

import (
	"testing"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

func bindingFor(endpointName, environment string) *openchoreov1alpha1.EndpointReleaseBinding {
	return &openchoreov1alpha1.EndpointReleaseBinding{
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner:       openchoreov1alpha1.EndpointReleaseBindingOwner{EndpointName: endpointName},
			Environment: environment,
		},
	}
}

func TestMakeRenderedReleaseName(t *testing.T) {
	// A name that already fits a label value keeps the name it always had.
	short := bindingFor("greeter-service-primary-endpoint-1", "development")
	if got, want := makeRenderedReleaseName(short),
		dpkubernetes.GenerateK8sName("e_greeter-service-primary-endpoint-1", "development"); got != want {
		t.Errorf("short name changed: got %q, want %q", got, want)
	}

	// A release line's Endpoint name is 63 characters on its own.
	long := bindingFor("go-greeter-source-workload-descriptor-v1alpha2-greeter-f4c739b8", "development")
	got := makeRenderedReleaseName(long)
	if len(got) > dpkubernetes.MaxLabelNameLength {
		t.Errorf("name %q is %d characters, longer than a label value", got, len(got))
	}
	if other := makeRenderedReleaseName(bindingFor("go-greeter-source-workload-descriptor-v1alpha2-greeter-8053be7a", "development")); other == got {
		t.Errorf("two Endpoints share the RenderedRelease name %q", got)
	}
}
