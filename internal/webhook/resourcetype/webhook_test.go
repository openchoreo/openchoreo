// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resourcetype

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// validTemplate returns a minimal RawExtension for a ConfigMap manifest.
// The resourcetype validator's contract is exhaustively covered in
// internal/validation/resource/resourcetype_test.go; the webhook tests
// only verify dispatch through ValidateCreate/Update/Delete.
func validTemplate() *runtime.RawExtension {
	return &runtime.RawExtension{
		Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"smoke"}}`),
	}
}

var _ = Describe("ResourceType Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.ResourceType
		oldObj    *openchoreodevv1alpha1.ResourceType
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.ResourceType{}
		oldObj = &openchoreodevv1alpha1.ResourceType{}
		validator = Validator{}
	})

	Context("ValidateCreate", func() {
		It("admits a minimal valid spec", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: validTemplate()},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects readyWhen referencing an undeclared id", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{
					ID:        "claim",
					ReadyWhen: "${applied.bogus.status.ready}",
					Template:  validTemplate(),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("readyWhen"))
		})

		It("rejects template referencing applied.*", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{
					ID: "claim",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"kind":"X","data":{"host":"${applied.claim.status.host}"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("applied"))
		})

		It("returns an error for non-ResourceType objects", func() {
			_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.ClusterResourceType{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ResourceType"))
		})
	})

	Context("ValidateUpdate", func() {
		It("admits a valid update", func() {
			oldObj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: validTemplate()},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: validTemplate()},
			}
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("rejects an update that introduces an undeclared applied id in readyWhen", func() {
			oldObj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{ID: "claim", Template: validTemplate()},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTypeManifest{
				{
					ID:        "claim",
					ReadyWhen: "${applied.bogus.status.ready}",
					Template:  validTemplate(),
				},
			}
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
		})

		It("returns an error for a non-ResourceType new object", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, &openchoreodevv1alpha1.ClusterResourceType{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("ValidateDelete", func() {
		It("is a no-op", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("returns an error for non-ResourceType objects", func() {
			_, err := validator.ValidateDelete(ctx, &openchoreodevv1alpha1.ClusterResourceType{})
			Expect(err).To(HaveOccurred())
		})
	})
})
