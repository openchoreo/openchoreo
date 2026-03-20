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
		It("Should admit trait with valid parameters and environmentConfigs", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"mountPath":{"type":"string"}},"required":["mountPath"]}`),
				},
			}

			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"},"storageClass":{"type":"string"}},"required":["storageClass"]}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject trait with invalid environmentConfigs schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"mountPath":{"type":"string"}}}`),
				},
			}
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":"not-an-object"}`), // invalid: properties must be an object
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse environmentConfigs schema"))
		})

		It("Should admit trait with only environmentConfigs (no parameters)", func() {
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"},"storageClass":{"type":"string","default":"local-path"}}}`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject trait with malformed environmentConfigs YAML", func() {
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{

				OpenAPIV3Schema: &runtime.RawExtension{

					Raw: []byte(`{malformed yaml`),
				},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse environmentConfigs schema"))
		})
	})

	Context("OpenAPIV3Schema Support", func() {
		It("should admit trait with openAPIV3Schema parameters and environmentConfigs", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"mountPath":{"type":"string"}},"required":["mountPath"]}`),
				},
			}
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"},"storageClass":{"type":"string"}},"required":["storageClass"]}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit trait with only openAPIV3Schema environmentConfigs (no parameters)", func() {
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"},"storageClass":{"type":"string","default":"local-path"}}}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit openAPIV3Schema with $defs and $ref", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"ResourceQuantity":{"type":"object","properties":{"cpu":{"type":"string","default":"100m"},"memory":{"type":"string","default":"256Mi"}}}},"properties":{"resources":{"$ref":"#/$defs/ResourceQuantity","default":{}}}}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit openAPIV3Schema with vendor extensions", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"url":{"type":"string","x-openchoreo-ui":{"widget":"text"}}}}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject malformed openAPIV3Schema in parameters", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{not valid`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject malformed openAPIV3Schema in environmentConfigs", func() {
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{malformed yaml`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse environmentConfigs schema"))
		})

		It("should reject openAPIV3Schema with circular $ref", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"A":{"$ref":"#/$defs/B"},"B":{"$ref":"#/$defs/A"}},"properties":{"val":{"$ref":"#/$defs/A"}}}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should validate boolean validation rules with openAPIV3Schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"mountPath":{"type":"string"}},"required":["mountPath"]}`),
				},
			}
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"}}}`),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.mountPath != ''}", Message: "mountPath must not be empty"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject non-boolean validation rule with openAPIV3Schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"name":{"type":"string","default":"app"}}}`),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.name}", Message: "returns string not bool"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should validate creates templates with openAPIV3Schema on update", func() {
			oldObj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"}}}`),
				},
			}

			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"20Gi"}}}`),
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
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

		It("should reject template expression in creates kind field", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "${parameters.kind}", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind must be a literal value"))
		})

		It("should allow creates template with metadata.namespace set to ${metadata.namespace}", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc", "namespace": "${metadata.namespace}"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject creates template with hardcoded metadata.namespace", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc", "namespace": "hardcoded-ns"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("${metadata.namespace}"))
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

		It("should reject Deployment kind in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "extra-deploy"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
			Expect(err.Error()).To(ContainSubstring("Deployment"))
		})

		It("should reject StatefulSet kind in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": {"name": "extra-sts"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
			Expect(err.Error()).To(ContainSubstring("StatefulSet"))
		})

		It("should reject CronJob kind in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "batch/v1", "kind": "CronJob", "metadata": {"name": "extra-cron"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
			Expect(err.Error()).To(ContainSubstring("CronJob"))
		})

		It("should reject Job kind in creates template", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": "extra-job"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
			Expect(err.Error()).To(ContainSubstring("Job"))
		})

		It("should reject workload resource kind case-insensitively", func() {
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "deployment", "metadata": {"name": "extra"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
		})

		It("should reject workload resource in creates on update", func() {
			oldObj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "ConfigMap", "metadata": {"name": "test"}}`),
					},
				},
			}
			obj.Spec.Creates = []openchoreodevv1alpha1.TraitCreate{
				{
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "extra"}}`),
					},
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("traits must not create workload resources"))
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

	Context("Validation Rules CEL Validation", func() {
		It("should reject malformed CEL expression in validation rule", func() {
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.x +}", Message: "bad rule"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should reject non-boolean CEL expression in validation rule", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"name":{"type":"string","default":"app"}}}`),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.name}", Message: "returns string not bool"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should admit valid boolean validation rules", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"mountPath":{"type":"string"}},"required":["mountPath"]}`),
				},
			}

			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"size":{"type":"string","default":"10Gi"}}}`),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.mountPath != ''}", Message: "mountPath must not be empty"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
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
