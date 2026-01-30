// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package trait

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Trait Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.Trait
		oldObj    *openchoreodevv1alpha1.Trait
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.Trait{}
		oldObj = &openchoreodevv1alpha1.Trait{}
		validator = Validator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When creating or updating Trait under Validating Webhook", func() {
		It("Should admit trait with valid parameters and envOverrides", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.TraitSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"mountPath": "string"}`),
				},
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{"size": "string | default=10Gi", "storageClass": "string"}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject trait with invalid envOverrides schema syntax", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.TraitSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"mountPath": "string"}`),
				},
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{"size": "unknown-type"}`), // invalid type
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build structural schema"))
		})

		It("Should admit trait with only envOverrides (no parameters)", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.TraitSchema{
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{"size": "string | default=10Gi", "storageClass": "string | default=local-path"}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject trait with malformed envOverrides YAML", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.TraitSchema{
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{malformed yaml`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse envOverrides schema"))
		})
	})

	Context("Creates Template Structure Validation", func() {
		It("should admit valid creates with proper template structure", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "${metadata.name}-pvc"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject missing apiVersion in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})

		It("should reject missing kind in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "metadata": {"name": "test-pvc"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind is required"))
		})

		It("should reject missing metadata.name in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.name is required"))
		})

		It("should reject empty template in creates", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(``),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is required"))
		})

		It("should allow CEL expression in creates metadata.name", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "${metadata.name}-sidecar-config"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should validate creates templates on update", func() {
			// Valid old object
			oldObj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "test"}}`),
					},
				},
			}

			// New object with missing apiVersion
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"kind": "ConfigMap", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})
	})

	Context("Creates CEL Validation", func() {
		It("should reject malformed CEL expression in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test"}, "spec": {"resources": {"requests": {"storage": "${parameters.size +}"}}}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid CEL expression"))
		})
	})

	Context("Patches Validation", func() {
		It("should admit valid patches with proper structure", func() {
			obj.Spec.Patches = []openchoreodevv1alpha1.TraitPatch{
				{
					Target: openchoreodevv1alpha1.PatchTarget{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					Operations: []openchoreodevv1alpha1.JSONPatchOperation{
						{
							Op:    "add",
							Path:  "/spec/template/spec/volumes/-",
							Value: &runtime.RawExtension{Raw: []byte(`{"name": "data", "persistentVolumeClaim": {"claimName": "${metadata.name}-pvc"}}`)},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject malformed CEL expression in patches", func() {
			obj.Spec.Patches = []openchoreodevv1alpha1.TraitPatch{
				{
					Target: openchoreodevv1alpha1.PatchTarget{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					Operations: []openchoreodevv1alpha1.JSONPatchOperation{
						{
							Op:    "add",
							Path:  "/spec/template/spec/volumes/-",
							Value: &runtime.RawExtension{Raw: []byte(`{"name": "${parameters.name +}"}`)},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid CEL expression"))
		})

		It("should admit patches with CEL expression as entire value", func() {
			obj.Spec.Patches = []openchoreodevv1alpha1.TraitPatch{
				{
					Target: openchoreodevv1alpha1.PatchTarget{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					Operations: []openchoreodevv1alpha1.JSONPatchOperation{
						{
							Op:    "add",
							Path:  "/metadata/labels",
							Value: &runtime.RawExtension{Raw: []byte(`"${metadata.labels}"`)},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
