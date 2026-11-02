// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("EndpointRelease immutability", func() {
	It("refuses reordering the backends of a release", func() {
		ctx := context.Background()
		release := &openchoreov1alpha1.EndpointRelease{
			ObjectMeta: metav1.ObjectMeta{Name: "reorder", Namespace: "default"},
			Spec: openchoreov1alpha1.EndpointReleaseSpec{
				Owner: openchoreov1alpha1.EndpointReleaseOwner{ProjectName: "p", EndpointName: "reorder"},
				EndpointType: openchoreov1alpha1.EndpointReleaseEndpointType{
					Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
					Name: "t",
					Spec: openchoreov1alpha1.EndpointTypeSpec{
						Resources: []openchoreov1alpha1.ResourceTemplate{{
							ID:       "route",
							Template: &runtime.RawExtension{Raw: []byte(routeTemplate)},
						}},
					},
				},
				Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
				BackendRefs: []openchoreov1alpha1.EndpointBackendRef{
					componentBackend("stable", "greeter", "http", 1),
					componentBackend("canary", "greeter-v2", "http", 1),
				},
			},
		}
		Expect(k8sClient.Create(ctx, release)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, release) })

		refs := release.Spec.BackendRefs
		release.Spec.BackendRefs = []openchoreov1alpha1.EndpointBackendRef{refs[1], refs[0]}
		err := k8sClient.Update(ctx, release)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)
		Expect(err.Error()).To(ContainSubstring("including the order of backendRefs"))
	})
})
