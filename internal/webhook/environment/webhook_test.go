// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package environment

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const scanRunTemplate = `{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"workflow-sa"}}`

const scanSchema = `{"type":"object","properties":{"image":{"type":"string"},"ticket":{"type":"string"}}}`

func strPtr(s string) *string { return &s }

func clusterHookRef(name string) openchoreodevv1alpha1.HookRef {
	return openchoreodevv1alpha1.HookRef{Kind: openchoreodevv1alpha1.HookRefKindClusterHook, Name: name}
}

func environment(name string, hooks *openchoreodevv1alpha1.HookSet) *openchoreodevv1alpha1.Environment {
	return &openchoreodevv1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       openchoreodevv1alpha1.EnvironmentSpec{Hooks: hooks},
	}
}

var _ = Describe("Environment Webhook", func() {
	var validator Validator

	BeforeEach(func() {
		validator = Validator{}
	})

	Context("When validating with wrong object type", func() {
		It("should reject non-Environment object on create", func() {
			_, err := validator.ValidateCreate(ctx, &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected an Environment object"))
		})

		It("should reject non-Environment object on update", func() {
			_, err := validator.ValidateUpdate(ctx, environment("e", nil), &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected an Environment object"))
		})

		It("should reject non-Environment object on delete", func() {
			_, err := validator.ValidateDelete(ctx, &openchoreodevv1alpha1.Hook{})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected an Environment object"))
		})
	})

	Context("When validating hook bindings without an API client", func() {
		// Every environment that exists today has no hooks; they must keep applying.
		It("admits an environment without hooks with no warnings", func() {
			warnings, err := validator.ValidateCreate(ctx, environment("plain", nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())

			warnings, err = validator.ValidateUpdate(ctx, environment("plain", nil), environment("plain", nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects an invalid binding with the full field path", func() {
			env := environment("bad", &openchoreodevv1alpha1.HookSet{
				PostDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("scan"), OnFailure: openchoreodevv1alpha1.HookFailurePolicyBlock}},
			})
			_, err := validator.ValidateCreate(ctx, env)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.hooks.postDeploy[0].onFailure"))
		})

		// Binding names key status.gate and the retry annotation, so a name may
		// appear once across both phases.
		It("rejects a binding name used in both phases", func() {
			env := environment("dup", &openchoreodevv1alpha1.HookSet{
				PreDeploy:  []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("trivy")}},
				PostDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("trivy")}},
			})
			_, err := validator.ValidateCreate(ctx, env)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.hooks.postDeploy[0].name"))
		})

		It("admits deletion without error", func() {
			_, err := validator.ValidateDelete(ctx, environment("plain", nil))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When hooks are resolved through the API server", func() {
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

			chk := &openchoreodevv1alpha1.ClusterHook{
				ObjectMeta: metav1.ObjectMeta{Name: "trivy"},
				Spec: openchoreodevv1alpha1.HookSpec{
					WorkflowRef: &openchoreodevv1alpha1.WorkflowRef{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "scan-workflow"},
					Parameters: []openchoreodevv1alpha1.HookParameter{
						{Name: "image", From: "${deployment.workload.containers.main.image}"},
						{Name: "ticket", Required: true},
					},
				},
			}
			Expect(k8sClient.Create(ctx, chk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, chk) })

			hk := &openchoreodevv1alpha1.Hook{
				ObjectMeta: metav1.ObjectMeta{Name: "smoke", Namespace: "default"},
				Spec: openchoreodevv1alpha1.HookSpec{
					WorkflowRef: &openchoreodevv1alpha1.WorkflowRef{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "scan-workflow"},
					Parameters:  []openchoreodevv1alpha1.HookParameter{{Name: "image", Default: strPtr("busybox")}},
				},
			}
			Expect(k8sClient.Create(ctx, hk)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, hk) })
		})

		It("rejects a binding that omits a parameter the hook requires", func() {
			env := environment("missing-param", &openchoreodevv1alpha1.HookSet{
				PreDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("trivy")}},
			})
			_, err := validator.ValidateCreate(ctx, env)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`parameters[ticket]: Required`))
		})

		It("warns about an unknown hook and about a missing workflow plane, but admits the environment", func() {
			env := environment("warned", &openchoreodevv1alpha1.HookSet{
				PreDeploy: []openchoreodevv1alpha1.HookBinding{
					{Name: "scan", HookRef: clusterHookRef("trivy"), Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1"}`)}},
					{Name: "later", HookRef: clusterHookRef("not-yet")},
				},
				PostDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "smoke", HookRef: openchoreodevv1alpha1.HookRef{Name: "smoke"}, Mode: openchoreodevv1alpha1.HookModeAsync}},
			})
			warnings, err := validator.ValidateCreate(ctx, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ContainElement(ContainSubstring(`ClusterHook "not-yet" not found`)))
			// No ClusterWorkflowPlane "default" exists in envtest, so every resolved
			// Workflow hook has nowhere to run; that must be visible at admission.
			Expect(warnings).To(ContainElement(ContainSubstring("hook ClusterHook/trivy has no workflow plane")))
			Expect(warnings).To(ContainElement(ContainSubstring("hook Hook/smoke has no workflow plane")))
		})

		It("stops warning about the plane once a default ClusterWorkflowPlane exists", func() {
			plane := &openchoreodevv1alpha1.ClusterWorkflowPlane{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Spec: openchoreodevv1alpha1.ClusterWorkflowPlaneSpec{PlaneID: "default"}}
			Expect(k8sClient.Create(ctx, plane)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, plane) })

			env := environment("planed", &openchoreodevv1alpha1.HookSet{
				PreDeploy: []openchoreodevv1alpha1.HookBinding{
					{Name: "scan", HookRef: clusterHookRef("trivy"), Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1"}`)}},
				},
			})
			warnings, err := validator.ValidateCreate(ctx, env)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("is enforced by the API server", func() {
			bad := environment("api-bad", &openchoreodevv1alpha1.HookSet{
				PreDeploy: []openchoreodevv1alpha1.HookBinding{{Name: "scan", HookRef: clusterHookRef("trivy")}},
			})
			err := k8sClient.Create(ctx, bad)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ticket"))

			good := environment("api-good", &openchoreodevv1alpha1.HookSet{
				PreDeploy: []openchoreodevv1alpha1.HookBinding{
					{Name: "scan", HookRef: clusterHookRef("trivy"), Parameters: &runtime.RawExtension{Raw: []byte(`{"ticket":"OPS-1"}`)}},
				},
			})
			Expect(k8sClient.Create(ctx, good)).To(Succeed())
			DeferCleanup(func() { _ = k8sClient.Delete(ctx, good) })
		})
	})
})
