// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clusterworkflow

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterWorkflow Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.ClusterWorkflow
		oldObj    *openchoreodevv1alpha1.ClusterWorkflow
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.ClusterWorkflow{
			Spec: openchoreodevv1alpha1.ClusterWorkflowSpec{
				WorkflowPlaneRef: &openchoreodevv1alpha1.ClusterWorkflowPlaneRef{
					Kind: openchoreodevv1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane,
					Name: "default",
				},
				RunTemplate: &runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"workflow-sa"}}`),
				},
			},
		}
		oldObj = &openchoreodevv1alpha1.ClusterWorkflow{}
		validator = Validator{}
		defaulter = Defaulter{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When creating ClusterWorkflow under Validating Webhook", func() {
		It("Should admit a valid ClusterWorkflow", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject ClusterWorkflow with missing metadata.namespace in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace"))
		})

		It("Should reject ClusterWorkflow with wrong metadata.namespace in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"wrong-ns"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace must be set to"))
		})

		It("Should reject ClusterWorkflow with missing apiVersion in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})
	})

	Context("ClusterWorkflow scoping constraint", func() {
		It("Should admit ClusterWorkflow with ClusterWorkflowPlane ref", func() {
			obj.Spec.WorkflowPlaneRef = &openchoreodevv1alpha1.ClusterWorkflowPlaneRef{
				Kind: openchoreodevv1alpha1.ClusterWorkflowPlaneRefKindClusterWorkflowPlane,
				Name: "default",
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should admit ClusterWorkflow with nil workflowPlaneRef (uses default)", func() {
			obj.Spec.WorkflowPlaneRef = nil
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When validating resources", func() {
		It("Should reject duplicate resource IDs", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.WorkflowResource{
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"secret-1","namespace":"${metadata.namespace}"}}`),
					},
				},
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"secret-2","namespace":"${metadata.namespace}"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})
	})

	Context("When validating externalRefs", func() {
		It("Should reject duplicate externalRef IDs", func() {
			obj.Spec.ExternalRefs = []openchoreodevv1alpha1.ExternalRef{
				{ID: "git-creds", APIVersion: "v1", Kind: "SecretReference", Name: "my-secret"},
				{ID: "git-creds", APIVersion: "v1", Kind: "SecretReference", Name: "other-secret"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})
	})

	Context("When updating ClusterWorkflow", func() {
		It("Should validate the new object on update", func() {
			newObj := obj.DeepCopy()
			newObj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test"}}`),
			}
			_, err := validator.ValidateUpdate(ctx, obj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace"))
		})

		It("Should return an error when newObj is not a ClusterWorkflow", func() {
			wrongObj := &openchoreodevv1alpha1.Workflow{}
			_, err := validator.ValidateUpdate(ctx, obj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterWorkflow object for the newObj"))
		})
	})

	Context("When validating ClusterWorkflow with wrong workflowPlaneRef kind", func() {
		It("Should reject ClusterWorkflow referencing a namespace-scoped WorkflowPlane", func() {
			obj.Spec.WorkflowPlaneRef = &openchoreodevv1alpha1.ClusterWorkflowPlaneRef{
				Kind: "WorkflowPlane",
				Name: "default",
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ClusterWorkflow can only reference ClusterWorkflowPlane"))
		})
	})

	Context("When creating ClusterWorkflow with wrong type", func() {
		It("Should return an error when given a non-ClusterWorkflow object on create", func() {
			wrongObj := &openchoreodevv1alpha1.Workflow{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterWorkflow object but got"))
		})
	})

	Context("When defaulting ClusterWorkflow", func() {
		It("Should return an error when given a non-ClusterWorkflow object on default", func() {
			wrongObj := &openchoreodevv1alpha1.Workflow{}
			err := defaulter.Default(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterWorkflow object but got"))
		})

		It("Should inject serviceAccountName into runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"},"spec":{"entrypoint":"build"}}`),
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).ToNot(HaveOccurred())

			var raw map[string]any
			err = json.Unmarshal(obj.Spec.RunTemplate.Raw, &raw)
			Expect(err).ToNot(HaveOccurred())

			spec, ok := raw["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(spec["serviceAccountName"]).To(Equal("workflow-sa"))
		})

		It("Should overwrite existing serviceAccountName", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"old-sa"}}`),
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).ToNot(HaveOccurred())

			var raw map[string]any
			err = json.Unmarshal(obj.Spec.RunTemplate.Raw, &raw)
			Expect(err).ToNot(HaveOccurred())

			spec, ok := raw["spec"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(spec["serviceAccountName"]).To(Equal("workflow-sa"))
		})
	})
})
