// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterTrait Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.ClusterTrait
		oldObj    *openchoreodevv1alpha1.ClusterTrait
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.ClusterTrait{}
		oldObj = &openchoreodevv1alpha1.ClusterTrait{}
		validator = Validator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// TODO (user): Add any teardown logic common to all tests
	})

	Context("When validating with wrong object type", func() {
		It("should reject non-ClusterTrait object on create", func() {
			wrongObj := &openchoreodevv1alpha1.Trait{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterTrait object"))
		})

		It("should reject non-ClusterTrait object on update", func() {
			wrongObj := &openchoreodevv1alpha1.Trait{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterTrait object"))
		})

		It("should reject non-ClusterTrait object on delete", func() {
			wrongObj := &openchoreodevv1alpha1.Trait{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ClusterTrait object"))
		})
	})

	Context("When deleting ClusterTrait", func() {
		It("should admit deletion without error", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("When creating ClusterTrait with nil template in creates", func() {
		It("should reject nil template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: nil,
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is required"))
		})
	})

	Context("When creating or updating ClusterTrait under Validating Webhook", func() {
		It("Should admit cluster trait with valid parameters and envOverrides", func() {
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

		It("Should reject cluster trait with invalid envOverrides schema syntax", func() {
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

		It("Should admit cluster trait with only envOverrides (no parameters)", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.TraitSchema{
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{"size": "string | default=10Gi", "storageClass": "string | default=local-path"}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject cluster trait with malformed envOverrides YAML", func() {
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
