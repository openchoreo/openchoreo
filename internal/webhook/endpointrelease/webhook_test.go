// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package endpointrelease

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("EndpointRelease Webhook", func() {
	var (
		ctx       context.Context
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = Validator{}
	})

	// newRelease returns a release whose type declares a required string
	// parameter "host", and whose template renders the given metadata.name.
	newRelease := func(name string, params string) *openchoreodevv1alpha1.EndpointRelease {
		release := &openchoreodevv1alpha1.EndpointRelease{
			Spec: openchoreodevv1alpha1.EndpointReleaseSpec{
				Owner: openchoreodevv1alpha1.EndpointReleaseOwner{ProjectName: "p", EndpointName: "e"},
				EndpointType: openchoreodevv1alpha1.EndpointReleaseEndpointType{
					Kind: openchoreodevv1alpha1.EndpointTypeRefKindClusterEndpointType,
					Name: "http",
					Spec: openchoreodevv1alpha1.EndpointTypeSpec{
						Parameters: &openchoreodevv1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(
							`{"type":"object","required":["host"],"properties":{"host":{"type":"string"}}}`)}},
						Resources: []openchoreodevv1alpha1.ResourceTemplate{{
							ID: "route",
							Template: &runtime.RawExtension{Raw: []byte(
								`{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"` + name + `"}}`)},
						}},
					},
				},
			},
		}
		if params != "" {
			release.Spec.Parameters = &runtime.RawExtension{Raw: []byte(params)}
		}
		return release
	}

	It("admits a release whose parameters match the snapshot schema", func() {
		_, err := validator.ValidateCreate(ctx, newRelease("${metadata.name}", `{"host":"api.example.com"}`))
		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects parameters that do not match the snapshot schema", func() {
		_, err := validator.ValidateCreate(ctx, newRelease("${metadata.name}", `{"host":42}`))
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("spec.parameters"))
		Expect(err.Error()).To(ContainSubstring(`ClusterEndpointType "http"`))
	})

	It("rejects missing required parameters", func() {
		_, err := validator.ValidateCreate(ctx, newRelease("${metadata.name}", ""))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("host"))
	})

	It("re-validates the embedded type spec", func() {
		_, err := validator.ValidateCreate(ctx, newRelease("${workload.name}", `{"host":"h"}`))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("spec.endpointType.spec.resources[0].template"))
	})

	It("leaves updates to the CRD's immutability rule", func() {
		release := newRelease("${workload.name}", `{"host":42}`)
		_, err := validator.ValidateUpdate(ctx, release, release)
		Expect(err).ToNot(HaveOccurred())
	})
})
