// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterauthzrolebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterAuthzRoleBinding RoleMapping XValidation", func() {
	bindingWithScope := func(name string, scope openchoreodevv1alpha1.ClusterTargetScope) *openchoreodevv1alpha1.ClusterAuthzRoleBinding {
		return &openchoreodevv1alpha1.ClusterAuthzRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec: openchoreodevv1alpha1.ClusterAuthzRoleBindingSpec{
				Entitlement: openchoreodevv1alpha1.EntitlementClaim{Claim: "groups", Value: "platform-engineers"},
				RoleMappings: []openchoreodevv1alpha1.ClusterRoleMapping{
					{
						RoleRef: openchoreodevv1alpha1.RoleRef{
							Kind: openchoreodevv1alpha1.RoleRefKindClusterAuthzRole,
							Name: "viewer",
						},
						Scope: scope,
					},
				},
			},
		}
	}

	Context("Create", func() {
		It("rejects scope.resource without scope.project", func() {
			obj := bindingWithScope("xv-resource-no-project", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Resource:  "r1",
			})
			err := k8sClient.Create(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scope.resource requires scope.project"))
		})

		It("rejects scope.component and scope.resource together", func() {
			obj := bindingWithScope("xv-component-and-resource", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
				Component: "c1",
				Resource:  "r1",
			})
			err := k8sClient.Create(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scope.component and scope.resource are mutually exclusive"))
		})

		It("admits scope.resource with scope.project", func() {
			obj := bindingWithScope("xv-resource-with-project", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
				Resource:  "r1",
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		})

		It("admits scope.component with scope.project", func() {
			obj := bindingWithScope("xv-component-with-project", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
				Component: "c1",
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		})
	})

	Context("Update", func() {
		It("rejects updating to scope.resource without scope.project", func() {
			obj := bindingWithScope("xv-update-resource-no-project", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			obj.Spec.RoleMappings[0].Scope = openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Resource:  "r1",
			}
			err := k8sClient.Update(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scope.resource requires scope.project"))
		})

		It("rejects updating to scope.component and scope.resource together", func() {
			obj := bindingWithScope("xv-update-component-and-resource", openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
				Resource:  "r1",
			})
			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })

			obj.Spec.RoleMappings[0].Scope = openchoreodevv1alpha1.ClusterTargetScope{
				Namespace: "ns1",
				Project:   "p1",
				Component: "c1",
				Resource:  "r1",
			}
			err := k8sClient.Update(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("scope.component and scope.resource are mutually exclusive"))
		})
	})
})
