// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Component Webhook", func() {
	var (
		obj       *openchoreodevv1alpha1.Component
		oldObj    *openchoreodevv1alpha1.Component
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		obj = &openchoreodevv1alpha1.Component{}
		oldObj = &openchoreodevv1alpha1.Component{}
		validator = Validator{}
		defaulter = Defaulter{}
	})

	componentWithTraits := func(traits []openchoreodevv1alpha1.ComponentTrait) *openchoreodevv1alpha1.Component {
		c := &openchoreodevv1alpha1.Component{}
		c.Spec.Traits = traits
		return c
	}

	Context("Defaulter webhook", func() {
		It("should return nil for a valid Component (no-op defaulter)", func() {
			err := defaulter.Default(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

	})

	Context("ValidateCreate", func() {
		It("should admit a Component with no traits", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit a Component with a valid DNS-1035 name", func() {
			obj.Name = "my-valid-component"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit a Component with a single-character lowercase name", func() {
			obj.Name = "a"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Component whose name contains dots (e.g. wso2-is-7.1.0)", func() {
			obj.Name = "wso2-is-7.1.0"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DNS-1035"))
		})

		It("should reject a Component whose name starts with a digit", func() {
			obj.Name = "1invalid"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DNS-1035"))
		})

		It("should reject a Component whose name contains uppercase letters", func() {
			obj.Name = "MyComponent"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DNS-1035"))
		})

		It("should reject a Component whose name ends with a hyphen", func() {
			obj.Name = "invalid-"
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DNS-1035"))
		})

		It("should admit a Component with unique trait instance names", func() {
			obj = componentWithTraits([]openchoreodevv1alpha1.ComponentTrait{
				{Name: "sidecar", InstanceName: "sidecar-a"},
				{Name: "sidecar", InstanceName: "sidecar-b"},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a Component with duplicate trait instance names", func() {
			obj = componentWithTraits([]openchoreodevv1alpha1.ComponentTrait{
				{Name: "sidecar", InstanceName: "my-sidecar"},
				{Name: "other-trait", InstanceName: "my-sidecar"},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("my-sidecar"))
		})

		It("should return an error when given a non-Component object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Component object"))
		})
	})

	Context("ValidateUpdate", func() {
		It("should admit a valid update with no traits", func() {
			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit an update with a valid DNS-1035 name", func() {
			newObj := &openchoreodevv1alpha1.Component{}
			newObj.Name = "valid-name-123"
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject an update whose name contains dots (e.g. wso2-is-7.1.0)", func() {
			newObj := &openchoreodevv1alpha1.Component{}
			newObj.Name = "wso2-is-7.1.0"
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("DNS-1035"))
		})

		It("should admit an update with unique trait instance names in the new object", func() {
			newObj := componentWithTraits([]openchoreodevv1alpha1.ComponentTrait{
				{Name: "trait-a", InstanceName: "instance-1"},
				{Name: "trait-b", InstanceName: "instance-2"},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject an update introducing duplicate trait instance names", func() {
			newObj := componentWithTraits([]openchoreodevv1alpha1.ComponentTrait{
				{Name: "trait-a", InstanceName: "dup"},
				{Name: "trait-b", InstanceName: "dup"},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("dup"))
		})

		It("should return an error when oldObj is not a Component", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateUpdate(ctx, wrongObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Component object for the oldObj"))
		})

		It("should return an error when newObj is not a Component", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Component object for the newObj"))
		})
	})

	Context("ValidateDelete", func() {
		It("should admit deletion of a valid Component", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-Component object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Component object"))
		})
	})
})
