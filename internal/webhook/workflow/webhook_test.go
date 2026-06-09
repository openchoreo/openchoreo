// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Workflow Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.Workflow
		oldObj    *openchoreodevv1alpha1.Workflow
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.Workflow{
			Spec: openchoreodevv1alpha1.WorkflowSpec{
				RunTemplate: &runtime.RawExtension{
					Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"${metadata.workflowRunName}","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"workflow-sa"}}`),
				},
			},
		}
		oldObj = &openchoreodevv1alpha1.Workflow{}
		validator = Validator{}
		defaulter = Defaulter{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	Context("When creating Workflow under Validating Webhook", func() {
		It("Should return an error when given a non-Workflow object on create", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterWorkflow{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Workflow object but got"))
		})

		It("Should admit a valid workflow with correct runTemplate", func() {
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject workflow with missing metadata.namespace in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace"))
		})

		It("Should reject workflow with wrong metadata.namespace in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"wrong-ns"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace must be set to"))
		})

		It("Should reject workflow with missing apiVersion in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})

		It("Should reject workflow with missing kind in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","metadata":{"name":"test","namespace":"${metadata.namespace}"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind is required"))
		})

		It("Should reject workflow with missing metadata.name in runTemplate", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"namespace":"${metadata.namespace}"}}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.name is required"))
		})
	})

	Context("When validating resources", func() {
		It("Should admit valid resources with correct namespace", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.WorkflowResource{
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"test-secret","namespace":"${metadata.namespace}"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject resources with missing namespace", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.WorkflowResource{
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"test-secret"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace"))
		})

		It("Should reject resources with wrong namespace", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.WorkflowResource{
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"test-secret","namespace":"other-ns"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace must be set to"))
		})

		It("Should reject resources with missing apiVersion", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.WorkflowResource{
				{
					ID: "git-secret",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"kind":"Secret","metadata":{"name":"test-secret","namespace":"${metadata.namespace}"}}`),
					},
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})

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
		It("Should admit unique externalRef IDs", func() {
			obj.Spec.ExternalRefs = []openchoreodevv1alpha1.ExternalRef{
				{ID: "git-creds", APIVersion: "v1", Kind: "SecretReference", Name: "my-secret"},
				{ID: "registry-creds", APIVersion: "v1", Kind: "SecretReference", Name: "registry-secret"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

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

	Context("When validating parameters schema", func() {
		It("Should admit valid parameters schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject invalid parameters schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{malformed`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("Should reject parameters schema with typo 'types' instead of 'type'", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"types":"object","properties":{"url":{"type":"string"}}}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown or invalid fields"))
		})

		It("Should reject parameters schema with unknown nested field", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"url":{"types":"string"}}}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown or invalid fields"))
		})
	})

	Context("When updating Workflow under Validating Webhook", func() {
		It("Should return an error when given a non-Workflow newObj on update", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterWorkflow{}
			_, err := validator.ValidateUpdate(ctx, obj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Workflow object for the newObj but got"))
		})

		It("Should validate the new object on update", func() {
			newObj := obj.DeepCopy()
			newObj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test"}}`),
			}
			_, err := validator.ValidateUpdate(ctx, obj, newObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.namespace"))
		})
	})

	Context("When defaulting Workflow", func() {
		It("Should return an error when given a non-Workflow object on default", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterWorkflow{}
			err := defaulter.Default(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Workflow object but got"))
		})

		It("Should return error when runTemplate has invalid JSON", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{bad json}`),
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to unmarshal runTemplate"))
		})

		It("Should return error when runTemplate spec is not an object", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"spec": "not-an-object"}`),
			}
			err := defaulter.Default(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec is"))
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
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"},"spec":{"serviceAccountName":"old-sa","entrypoint":"build"}}`),
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

		It("Should create spec if missing and inject serviceAccountName", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{"apiVersion":"argoproj.io/v1alpha1","kind":"Workflow","metadata":{"name":"test","namespace":"${metadata.namespace}"}}`),
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

		It("Should handle nil runTemplate gracefully", func() {
			obj.Spec.RunTemplate = nil
			err := defaulter.Default(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When deleting Workflow under Validating Webhook", func() {
		It("Should admit deletion of a valid Workflow", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should return an error when given a non-Workflow object on delete", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterWorkflow{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a Workflow object but got"))
		})
	})

	Context("When runTemplate has malformed JSON", func() {
		It("Should reject workflow with malformed runTemplate JSON", func() {
			obj.Spec.RunTemplate = &runtime.RawExtension{
				Raw: []byte(`{bad json}`),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
		})
	})
})
