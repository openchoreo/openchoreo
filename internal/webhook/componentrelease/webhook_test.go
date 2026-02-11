// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const workloadTypeDeployment = "deployment"

var _ = Describe("ComponentRelease Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.ComponentRelease
		oldObj    *openchoreodevv1alpha1.ComponentRelease
		validator Validator
		defaulter Defaulter
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.ComponentRelease{}
		oldObj = &openchoreodevv1alpha1.ComponentRelease{}
		validator = Validator{}
		defaulter = Defaulter{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	// Helper to create a valid deployment template
	validDeploymentTemplate := func() *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
		}
	}

	// Helper to create a deployment template with CEL expressions
	deploymentTemplateWithCEL := func(celExpr string) *runtime.RawExtension {
		return &runtime.RawExtension{
			Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}, "spec": {"replicas": "` + celExpr + `"}}`),
		}
	}

	// Helper to create a valid base ComponentRelease
	validComponentRelease := func() *openchoreodevv1alpha1.ComponentRelease {
		return &openchoreodevv1alpha1.ComponentRelease{
			Spec: openchoreodevv1alpha1.ComponentReleaseSpec{
				Owner: openchoreodevv1alpha1.ComponentReleaseOwner{
					ProjectName:   "test-project",
					ComponentName: "test-component",
				},
				ComponentType: openchoreodevv1alpha1.ComponentTypeSpec{
					WorkloadType: workloadTypeDeployment,
					Resources: []openchoreodevv1alpha1.ResourceTemplate{
						{
							ID:       "deployment",
							Template: validDeploymentTemplate(),
						},
					},
				},
				ComponentProfile: &openchoreodevv1alpha1.ComponentProfile{},
				Workload: openchoreodevv1alpha1.WorkloadTemplateSpec{
					Containers: map[string]openchoreodevv1alpha1.Container{
						"app": {
							Image: "nginx:latest",
						},
					},
				},
			},
		}
	}

	Context("Happy Path Tests", func() {
		It("should admit valid ComponentRelease with matching workload resource", func() {
			obj = validComponentRelease()

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit valid ComponentRelease with parameters schema and values", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}
			obj.Spec.ComponentProfile.Parameters = &runtime.RawExtension{
				Raw: []byte(`{"replicas": 3}`),
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit ComponentRelease when parameter with default is omitted", func() {
			// Fields with "default=" in the schema are NOT marked as required in the generated JSON Schema.
			// This means omitting them during validation is valid. The actual defaults are applied later
			// during rendering in the ReleaseBinding controller, not during webhook validation.
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1", "image": "string | default=nginx"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}
			// Omit parameters entirely - validation passes because fields with defaults are optional
			obj.Spec.ComponentProfile.Parameters = nil

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit ComponentRelease with partial parameters when others have defaults", func() {
			// "name" has no default so it's required; "replicas" has a default so it's optional
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1", "name": "string"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
			// Only provide required field ("name"), omit optional field with default ("replicas")
			obj.Spec.ComponentProfile.Parameters = &runtime.RawExtension{
				Raw: []byte(`{"name": "my-app"}`),
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit ComponentRelease with valid trait instance parameters", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Schema: openchoreodevv1alpha1.TraitSchema{
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"mountPath": "string", "size": "string | default=10Gi"}`),
						},
					},
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
							},
						},
					},
				},
			}
			obj.Spec.ComponentProfile.Traits = []openchoreodevv1alpha1.ComponentTrait{
				{
					Name:         "storage",
					InstanceName: "data-storage",
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"mountPath": "/data"}`), // size has default, can be omitted
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Parameter Schema Validation", func() {
		It("should reject when required parameter is missing", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer", "name": "string"}`), // no defaults, both required
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
			// Only provide one required field
			obj.Spec.ComponentProfile.Parameters = &runtime.RawExtension{
				Raw: []byte(`{"replicas": 3}`),
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
		})

		It("should reject when parameter has wrong type", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
			// Provide string instead of integer
			obj.Spec.ComponentProfile.Parameters = &runtime.RawExtension{
				Raw: []byte(`{"replicas": "not-a-number"}`),
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("replicas"))
		})

		It("should reject when trait instance parameter is missing required field", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Schema: openchoreodevv1alpha1.TraitSchema{
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"mountPath": "string", "size": "string"}`), // both required
						},
					},
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
							},
						},
					},
				},
			}
			obj.Spec.ComponentProfile.Traits = []openchoreodevv1alpha1.ComponentTrait{
				{
					Name:         "storage",
					InstanceName: "data-storage",
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"mountPath": "/data"}`), // missing size
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("size"))
		})

		It("should reject when trait referenced in componentProfile doesn't exist in traits", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{}
			obj.Spec.ComponentProfile.Traits = []openchoreodevv1alpha1.ComponentTrait{
				{
					Name:         "nonexistent-trait",
					InstanceName: "instance1",
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found in traits snapshot"))
		})

		It("should reject duplicate trait instance names", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
							},
						},
					},
				},
			}
			obj.Spec.ComponentProfile.Traits = []openchoreodevv1alpha1.ComponentTrait{
				{
					Name:         "storage",
					InstanceName: "duplicate-name",
				},
				{
					Name:         "storage",
					InstanceName: "duplicate-name", // duplicate
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate-name"))
		})
	})

	Context("CEL Validation in Embedded ComponentType", func() {
		It("should reject malformed CEL expression in ComponentType resource template", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}, "spec": {"replicas": "${parameters.replicas +}"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid CEL expression"))
		})

		It("should reject forEach not wrapped in ${...} in ComponentType", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:      "deployment",
					ForEach: "parameters.items", // missing ${...}
					Var:     "item",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forEach must be wrapped in ${...}"))
		})

		It("should reject includeWhen not wrapped in ${...} in ComponentType", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:          "deployment",
					IncludeWhen: "parameters.enabled", // missing ${...}
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("includeWhen must be wrapped in ${...}"))
		})

		It("should admit valid CEL expressions in ComponentType", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1", "enabled": "boolean | default=true"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:          "deployment",
					IncludeWhen: "${parameters.enabled}",
					Template:    deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("CEL Validation in Embedded Traits", func() {
		It("should reject malformed CEL expression in Trait creates template", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test"}, "spec": {"resources": {"requests": {"storage": "${parameters.size +}"}}}}`),
							},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid CEL expression"))
		})

		It("should reject malformed CEL expression in Trait patches", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"sidecar": {
					Patches: []openchoreodevv1alpha1.TraitPatch{
						{
							Target: openchoreodevv1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
							},
							Operations: []openchoreodevv1alpha1.JSONPatchOperation{
								{
									Op:    "add",
									Path:  "/spec/template/spec/containers/-",
									Value: &runtime.RawExtension{Raw: []byte(`{"name": "${parameters.name +}"}`)},
								},
							},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid CEL expression"))
		})

		It("should admit valid CEL expressions in Trait creates and patches", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Schema: openchoreodevv1alpha1.TraitSchema{
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"size": "string | default=10Gi"}`),
						},
					},
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "${metadata.name}-pvc"}, "spec": {"resources": {"requests": {"storage": "${parameters.size}"}}}}`),
							},
						},
					},
					Patches: []openchoreodevv1alpha1.TraitPatch{
						{
							Target: openchoreodevv1alpha1.PatchTarget{
								Group:   "apps",
								Version: "v1",
								Kind:    "Deployment",
							},
							Operations: []openchoreodevv1alpha1.JSONPatchOperation{
								{
									Op:   "add",
									Path: "/spec/template/spec/volumes/-",
									Value: &runtime.RawExtension{
										Raw: []byte(`{"name": "data", "persistentVolumeClaim": {"claimName": "${metadata.name}-pvc"}}`),
									},
								},
							},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("CEL Validation in Validation Rules", func() {
		It("should reject malformed CEL expression in ComponentType validation rule", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.x +}", Message: "bad rule"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should reject non-boolean CEL expression in ComponentType validation rule", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"name": "string | default=app"}`),
				},
			}
			obj.Spec.ComponentType.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.name}", Message: "returns string not bool"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should admit valid validation rules in ComponentType and Traits", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
			}
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}
			obj.Spec.ComponentType.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
			}
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Schema: openchoreodevv1alpha1.TraitSchema{
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{"size": "integer | default=10"}`),
						},
					},
					Validations: []openchoreodevv1alpha1.ValidationRule{
						{Rule: "${parameters.size > 0}", Message: "size must be positive"},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Resource Structure Validation", func() {
		It("should reject missing apiVersion in ComponentType resource template", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})

		It("should reject missing kind in ComponentType resource template", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind is required"))
		})

		It("should reject missing metadata.name in ComponentType resource template", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata.name is required"))
		})

		It("should reject missing apiVersion in Trait creates template", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
							},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("apiVersion is required"))
		})

		It("should reject when no resource matches workloadType", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.WorkloadType = workloadTypeDeployment
			obj.Spec.ComponentType.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "service",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must have exactly one resource with kind matching workloadType"))
		})

		It("should reject when workload has no containers", func() {
			obj = validComponentRelease()
			obj.Spec.Workload.Containers = nil

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workload must have at least one container"))
		})

		It("should reject when workload has empty containers map", func() {
			obj = validComponentRelease()
			obj.Spec.Workload.Containers = map[string]openchoreodevv1alpha1.Container{}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("workload must have at least one container"))
		})
	})

	Context("Schema Parsing Failures", func() {
		It("should reject invalid JSON in ComponentType schema types", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Types: &runtime.RawExtension{
					Raw: []byte(`{malformed json`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse types"))
		})

		It("should reject invalid JSON in ComponentType schema parameters", func() {
			obj = validComponentRelease()
			obj.Spec.ComponentType.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{malformed`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject invalid JSON in Trait schema", func() {
			obj = validComponentRelease()
			obj.Spec.Traits = map[string]openchoreodevv1alpha1.TraitSpec{
				"storage": {
					Schema: openchoreodevv1alpha1.TraitSchema{
						Parameters: &runtime.RawExtension{
							Raw: []byte(`{malformed`),
						},
					},
					Creates: []openchoreodevv1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(`{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": {"name": "test-pvc"}}`),
							},
						},
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})
	})
})
