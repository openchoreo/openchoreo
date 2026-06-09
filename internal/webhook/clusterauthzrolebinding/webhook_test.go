// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/require"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

var _ = Describe("ClusterAuthzRoleBinding Webhook", func() {
	var (
		obj       *openchoreodevv1alpha1.ClusterAuthzRoleBinding
		oldObj    *openchoreodevv1alpha1.ClusterAuthzRoleBinding
		validator ClusterAuthzRoleBindingValidator
	)

	BeforeEach(func() {
		obj = &openchoreodevv1alpha1.ClusterAuthzRoleBinding{}
		oldObj = &openchoreodevv1alpha1.ClusterAuthzRoleBinding{}
		validator = ClusterAuthzRoleBindingValidator{}
	})

	bindingWithRoleMappings := func(mappings []openchoreodevv1alpha1.ClusterRoleMapping) *openchoreodevv1alpha1.ClusterAuthzRoleBinding {
		rb := &openchoreodevv1alpha1.ClusterAuthzRoleBinding{}
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
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.ClusterRoleMapping{{Conditions: nil}})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should admit a binding with valid conditions", func() {
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject a binding with an invalid condition and report the index", func() {
			obj = bindingWithRoleMappings([]openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition, invalidCondition}},
			})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("roleMappings[1].conditions[1]"))
		})

		It("should return an error when given a non-ClusterAuthzRoleBinding object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected ClusterAuthzRoleBinding"))
		})
	})

	Context("ValidateUpdate", func() {
		It("should admit an update with valid conditions in the new object", func() {
			newObj := bindingWithRoleMappings([]openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCondition}},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject an update introducing an invalid condition", func() {
			newObj := bindingWithRoleMappings([]openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{invalidCondition}},
			})
			_, err := validator.ValidateUpdate(ctx, oldObj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("roleMappings[0].conditions[0]"))
		})

		It("should return an error when newObj is not a ClusterAuthzRoleBinding", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected ClusterAuthzRoleBinding"))
		})
	})

	Context("ValidateDelete", func() {
		It("should admit deletion of a valid ClusterAuthzRoleBinding", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-ClusterAuthzRoleBinding object", func() {
			wrongObj := &openchoreodevv1alpha1.Project{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected ClusterAuthzRoleBinding"))
		})
	})
})

func TestValidateClusterRoleMappings(t *testing.T) {
	validCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{authzcore.ActionCreateReleaseBinding},
		Expression: `resource.environment == "prod"`,
	}
	emptyActionsCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{},
		Expression: `resource.environment == "prod"`,
	}
	emptyExprCond := openchoreodevv1alpha1.AuthzCondition{
		Actions:    []string{authzcore.ActionCreateReleaseBinding},
		Expression: "",
	}

	tests := []struct {
		name     string
		mappings []openchoreodevv1alpha1.ClusterRoleMapping
		wantErrs []string
	}{
		{
			name:     "nil mappings",
			mappings: nil,
		},
		{
			name:     "mapping with no conditions",
			mappings: []openchoreodevv1alpha1.ClusterRoleMapping{{Conditions: nil}},
		},
		{
			name: "mapping with valid condition",
			mappings: []openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond}},
			},
		},
		{
			name: "error path includes correct indices",
			mappings: []openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond, emptyActionsCond}},
			},
			wantErrs: []string{"spec.roleMappings[1].conditions[1]"},
		},
		{
			name: "multiple invalid conditions across mappings are all reported",
			mappings: []openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{emptyActionsCond}},
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{validCond, emptyExprCond}},
			},
			wantErrs: []string{
				"spec.roleMappings[0].conditions[0]",
				"spec.roleMappings[1].conditions[1]",
			},
		},
		{
			name: "multiple invalid conditions within same mapping are all reported",
			mappings: []openchoreodevv1alpha1.ClusterRoleMapping{
				{Conditions: []openchoreodevv1alpha1.AuthzCondition{emptyActionsCond, emptyExprCond}},
			},
			wantErrs: []string{
				"spec.roleMappings[0].conditions[0]",
				"spec.roleMappings[0].conditions[1]",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateClusterRoleMappings(tt.mappings)
			if len(tt.wantErrs) == 0 {
				require.Empty(t, errs)
			} else {
				require.Len(t, errs, len(tt.wantErrs))
				aggregate := errs.ToAggregate().Error()
				for _, wantErr := range tt.wantErrs {
					require.Contains(t, aggregate, wantErr)
				}
			}
		})
	}
}
