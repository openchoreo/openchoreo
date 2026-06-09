// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcereleasebinding

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ResourceRelease watch mapper", func() {
	mapperCtx := context.Background()

	newBinding := func(name, ns, releaseName string) *openchoreov1alpha1.ResourceReleaseBinding {
		return &openchoreov1alpha1.ResourceReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: openchoreov1alpha1.ResourceReleaseBindingSpec{
				Owner: openchoreov1alpha1.ResourceReleaseBindingOwner{
					ProjectName:  "p",
					ResourceName: "r",
				},
				Environment:     "dev",
				ResourceRelease: releaseName,
			},
		}
	}

	buildClient := func(objs ...client.Object) client.Client {
		s := runtime.NewScheme()
		Expect(openchoreov1alpha1.AddToScheme(s)).To(Succeed())
		return fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(objs...).
			WithIndex(&openchoreov1alpha1.ResourceReleaseBinding{},
				resourceReleaseRefIndex, indexResourceReleaseRef).
			Build()
	}

	It("enqueues only bindings in the same namespace whose spec.resourceRelease matches", func() {
		matching := newBinding("matches", "ns1", "r-abc123")
		differentRelease := newBinding("other-release", "ns1", "r-def456")
		differentNs := newBinding("other-ns", "ns2", "r-abc123")
		unsetPin := newBinding("unset-pin", "ns1", "")

		r := &Reconciler{Client: buildClient(matching, differentRelease, differentNs, unsetPin)}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		requests := r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)

		Expect(requests).To(HaveLen(1))
		Expect(requests[0].Namespace).To(Equal("ns1"))
		Expect(requests[0].Name).To(Equal("matches"))
	})

	It("returns no requests when no binding references the release", func() {
		r := &Reconciler{Client: buildClient(newBinding("a", "ns1", "r-other"))}
		release := &openchoreov1alpha1.ResourceRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "r-abc123", Namespace: "ns1"},
		}

		Expect(r.listResourceReleaseBindingsForResourceRelease(mapperCtx, release)).To(BeEmpty())
	})
})
