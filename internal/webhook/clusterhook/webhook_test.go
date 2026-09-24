// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterhook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const scanRunTemplate = `{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"workflow-sa"}}`

const scanSchema = `{"type":"object","properties":{"image":{"type":"string"}},"required":["image"]}`

var _ = Describe("ClusterHook Webhook", func() {
	var (
		obj       *openchoreodevv1alpha1.ClusterHook
		validator Validator
	)

	BeforeEach(func() {
		obj = &openchoreodevv1alpha1.ClusterHook{
			ObjectMeta: metav1.ObjectMeta{Name: "trivy-scan"},
			Spec: openchoreodevv1alpha1.HookSpec{
				WorkflowRef: &openchoreodevv1alpha1.WorkflowRef{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "cluster-scan"},
				Parameters:  []openchoreodevv1alpha1.HookParameter{{Name: "image", From: "${deployment.workload.containers.main.image}"}},
			},
		}
		validator = Validator{}
	})

	Context("When validating with wrong object type", func() {
		It("should reject non-ClusterHook object on create", func() {
			_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterHook object"))
		})

		It("should reject non-ClusterHook object on update", func() {
			_, err := validator.ValidateUpdate(ctx, obj, &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterHook object"))
		})

		It("should reject non-ClusterHook object on delete", func() {
			_, err := validator.ValidateDelete(ctx, &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterHook object"))
		})
	})

	Context("When validating the spec", func() {
		It("admits a ClusterWorkflow reference", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		// A ClusterHook has no namespace, so a namespaced Workflow could never be resolved.
		It("rejects a namespaced Workflow reference", func() {
			obj.Spec.WorkflowRef.Kind = openchoreodevv1alpha1.WorkflowRefKindWorkflow
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ClusterHook may only reference a ClusterWorkflow"))

			_, err = validator.ValidateUpdate(ctx, obj.DeepCopy(), obj)
			Expect(err).To(HaveOccurred())
		})

		It("admits deletion without error", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When the referenced ClusterWorkflow is looked up", func() {
		BeforeEach(func() {
			if k8sClient == nil {
				Skip("envtest apiserver not available")
			}
			validator = Validator{Client: k8sClient}
			cw := &openchoreodevv1alpha1.ClusterWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-scan"},
				Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
					RunTemplate: &runtime.RawExtension{Raw: []byte(scanRunTemplate)},
					Parameters:  &openchoreodevv1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(scanSchema)}},
				},
			}
			Expect(k8sClient.Create(ctx, cw)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cw) })
		})

		It("warns when the ClusterWorkflow does not exist", func() {
			obj.Spec.WorkflowRef.Name = "missing"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("missing"))
		})

		It("rejects an unmapped required input through the API server", func() {
			bad := obj.DeepCopy()
			bad.Spec.Parameters = nil
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`workflow input "image" is required`))

			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
		})
	})
})
