// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package hook

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// scanRunTemplate is the smallest runTemplate the ClusterWorkflow webhook admits.
const scanRunTemplate = `{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"workflow-sa"}}`

// scanSchema declares image (required) and severity (optional).
const scanSchema = `{"type":"object","properties":{"image":{"type":"string"},"severity":{"type":"string"}},"required":["image"]}`

func strPtr(s string) *string { return &s }

func validHookSpec(workflow string) openchoreodevv1alpha1.HookSpec {
	return openchoreodevv1alpha1.HookSpec{
		Type:        openchoreodevv1alpha1.HookTypeWorkflow,
		WorkflowRef: &openchoreodevv1alpha1.WorkflowRef{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: workflow},
		Parameters: []openchoreodevv1alpha1.HookParameter{
			{Name: "image", From: "${deployment.workload.containers.main.image}"},
			{Name: "severity", Default: strPtr("HIGH")},
		},
	}
}

var _ = Describe("Hook Webhook", func() {
	var (
		obj       *openchoreodevv1alpha1.Hook
		validator Validator
	)

	BeforeEach(func() {
		obj = &openchoreodevv1alpha1.Hook{
			ObjectMeta: metav1.ObjectMeta{Name: "scan", Namespace: "default"},
			Spec:       validHookSpec("scan-workflow"),
		}
		validator = Validator{}
	})

	Context("When validating with wrong object type", func() {
		It("should reject non-Hook object on create", func() {
			_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.ClusterHook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Hook object"))
		})

		It("should reject non-Hook object on update", func() {
			_, err := validator.ValidateUpdate(ctx, obj, &openchoreodevv1alpha1.ClusterHook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Hook object"))
		})

		It("should reject non-Hook object on delete", func() {
			_, err := validator.ValidateDelete(ctx, &openchoreodevv1alpha1.ClusterHook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Hook object"))
		})
	})

	Context("When validating the spec without an API client", func() {
		It("admits a valid spec with no warnings", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects a hook with no workflowRef because there is nothing to run", func() {
			obj.Spec.WorkflowRef = nil
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.workflowRef: Required"))
		})

		It("rejects a parameter whose from expression does not compile, on update too", func() {
			obj.Spec.Parameters[0].From = "${deployment.release +}"
			_, err := validator.ValidateUpdate(ctx, obj.DeepCopy(), obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("CEL compilation failed"))
		})

		It("admits deletion without error", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When the referenced workflow is looked up", func() {
		BeforeEach(func() {
			if k8sClient == nil {
				Skip("envtest apiserver not available")
			}
			validator = Validator{Client: k8sClient}
			cw := &openchoreodevv1alpha1.ClusterWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "scan-workflow"},
				Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
					RunTemplate: &runtime.RawExtension{Raw: []byte(scanRunTemplate)},
					Parameters:  &openchoreodevv1alpha1.SchemaSection{OpenAPIV3Schema: &runtime.RawExtension{Raw: []byte(scanSchema)}},
				},
			}
			Expect(k8sClient.Create(ctx, cw)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, cw) })
		})

		It("warns, but admits, when the workflow does not exist so hooks can be applied first", func() {
			obj.Spec.WorkflowRef.Name = "not-yet"
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("not-yet"))
			Expect(warnings[0]).To(ContainSubstring("not found"))
		})

		It("admits a hook whose parameters match the workflow inputs", func() {
			warnings, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects a parameter the workflow does not declare", func() {
			obj.Spec.Parameters = append(obj.Spec.Parameters, openchoreodevv1alpha1.HookParameter{Name: "shade", Required: true})
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`does not declare an input named "shade"`))
		})

		It("rejects a hook that leaves a required workflow input unmapped", func() {
			obj.Spec.Parameters = obj.Spec.Parameters[1:] // drop image
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`workflow input "image" is required but not mapped`))
		})

		It("is enforced by the API server", func() {
			bad := obj.DeepCopy()
			bad.Spec.Parameters = append(bad.Spec.Parameters, openchoreodevv1alpha1.HookParameter{Name: "shade", Required: true})
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("shade"))

			Expect(k8sClient.Create(ctx, obj)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, obj) })
		})
	})
})
