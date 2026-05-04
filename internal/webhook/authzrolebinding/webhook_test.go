// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package authzrolebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

var _ = Describe("AuthzRoleBinding Webhook", func() {
	var (
		obj       *openchoreodevv1alpha1.AuthzRoleBinding
		oldObj    *openchoreodevv1alpha1.AuthzRoleBinding
		validator AuthzRoleBindingValidator
	)

	BeforeEach(func() {
		obj = &openchoreodevv1alpha1.AuthzRoleBinding{}
		oldObj = &openchoreodevv1alpha1.AuthzRoleBinding{}
		validator = AuthzRoleBindingValidator{}
	})

	bindingWithRoleMappings := func(mappings []openchoreodevv1alpha1.RoleMapping) *openchoreodevv1alpha1.AuthzRoleBinding {
		rb := &openchoreodevv1alpha1.AuthzRoleBinding{}
		rb.Spec.RoleMappings = mappings
		return rb
	}

	validCondition := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{authzcore.ActionCreateReleaseBinding},
		Expression: `resource.environment == "prod"`,
	}
	invalidCondition := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{},
		Expression: `resource.environment == "prod"`,
	}

	Context("ValidateCreate", func() {
		It("should admit a binding with role mappings that have no conditions", func() {
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.RoleMapping{{Conditions: nil}})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit a binding with valid conditions", func() {
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a binding with an invalid condition and report the index", func() {
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{invalidCondition, validCondition}},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("roleMappings[1].conditions[0]"))
		})

		It("should return an error when given a non-AuthzRoleBinding object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected AuthzRoleBinding"))
		})
	})

	Context("ValidateUpdate", func() {
		It("should admit an update with valid conditions in the new object", func() {
			newObj := bindingWithRoleMappings([]openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject an update introducing an invalid condition", func() {
			newObj := bindingWithRoleMappings([]openchoreodevv1alpha1.RoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{invalidCondition}},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("roleMappings[0].conditions[0]"))
		})

		It("should return an error when newObj is not an AuthzRoleBinding", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected AuthzRoleBinding"))
		})
	})

	Context("ValidateDelete", func() {
		It("should admit deletion of a valid AuthzRoleBinding", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-AuthzRoleBinding object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected AuthzRoleBinding"))
		})
	})
})
