// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterendpointtype

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// The endpoint-type validator's contract is covered in
// internal/validation/endpoint/endpointtype_test.go; these only verify
// dispatch through ValidateCreate/Update/Delete and the error shape.
func route(name string) openchoreodevv1alpha1.ResourceTemplate {
	return openchoreodevv1alpha1.ResourceTemplate{
		ID: "route",
		Template: &runtime.RawExtension{Raw: []byte(
			`{"apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute","metadata":{"name":"` + name + `"}}`)},
	}
}

var _ = Describe("ClusterEndpointType Webhook", func() {
	var (
		ctx       context.Context
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		validator = Validator{}
	})

	newType := func(tmpl openchoreodevv1alpha1.ResourceTemplate) *openchoreodevv1alpha1.ClusterEndpointType {
		return &openchoreodevv1alpha1.ClusterEndpointType{
			Spec: openchoreodevv1alpha1.ClusterEndpointTypeSpec{Resources: []openchoreodevv1alpha1.ResourceTemplate{tmpl}},
		}
	}

	It("admits a valid spec on create and update", func() {
		obj := newType(route("${metadata.name}"))
		_, err := validator.ValidateCreate(ctx, obj)
		Expect(err).ToNot(HaveOccurred())
		_, err = validator.ValidateUpdate(ctx, obj.DeepCopy(), obj)
		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects a template referencing an unknown variable as Invalid", func() {
		obj := newType(route("${workload.name}"))
		_, err := validator.ValidateCreate(ctx, obj)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
		Expect(err.Error()).To(ContainSubstring("spec.resources[0].template"))
	})

	It("re-validates on update", func() {
		_, err := validator.ValidateUpdate(ctx, newType(route("${metadata.name}")), newType(route("${workload.name}")))
		Expect(err).To(HaveOccurred())
	})

	It("admits deletes", func() {
		_, err := validator.ValidateDelete(ctx, newType(route("${workload.name}")))
		Expect(err).ToNot(HaveOccurred())
	})

	It("rejects the wrong object type", func() {
		_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.EndpointType{})
		Expect(err).To(HaveOccurred())
	})
})
