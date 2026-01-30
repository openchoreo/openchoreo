// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const workloadTypeDeployment = "deployment"

var _ = Describe("ComponentType Webhook", func() {
	var (
		ctx       context.Context
		obj       *openchoreodevv1alpha1.ComponentType
		oldObj    *openchoreodevv1alpha1.ComponentType
		validator Validator
	)

	BeforeEach(func() {
		ctx = context.Background()
		obj = &openchoreodevv1alpha1.ComponentType{}
		oldObj = &openchoreodevv1alpha1.ComponentType{}
		validator = Validator{}
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

	Context("Happy Path Tests", func() {
		It("should admit valid ComponentType with parameters and matching workload resource", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit valid ComponentType with parameters and envOverrides", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=1"}`),
				},
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`{"image": "string"}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit valid update with same validation as create", func() {
			// Set up valid oldObj
			oldObj.Spec.WorkloadType = workloadTypeDeployment
			oldObj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			// Set up valid newObj
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer | default=2"}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			_, err := validator.ValidateUpdate(ctx, oldObj, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Schema Parsing Failures", func() {
		BeforeEach(func() {
			// Set up valid base ComponentType
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
		})

		It("should reject invalid JSON in spec.schema.types", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Types: &runtime.RawExtension{
					Raw: []byte(`{malformed json`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse types"))
		})

		It("should reject invalid JSON in spec.schema.parameters", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{malformed`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject invalid JSON in spec.schema.envOverrides", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				EnvOverrides: &runtime.RawExtension{
					Raw: []byte(`not valid yaml`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse envOverrides schema"))
		})
	})

	Context("Structural Schema Build Failures", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
		})

		It("should reject unknown shorthand type in parameters", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"field": "unknown-type"}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build structural schema"))
		})

		It("should reject invalid type reference in parameters", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Types: &runtime.RawExtension{
					Raw: []byte(`{"Database": {"host": "string", "port": "integer"}}`),
				},
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"db": "NonExistent"}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to build structural schema"))
		})
	})

	Context("Resource CEL/JSON Validation Errors", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
		})

		It("should reject malformed CEL expression in template", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
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

		It("should reject invalid JSON in resource template", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{invalid json`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid JSON"))
		})

		It("should reject forEach not wrapped in ${...}", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:      "deployment",
					ForEach: "parameters.items",
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

		It("should reject includeWhen not wrapped in ${...}", func() {
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:          "deployment",
					IncludeWhen: "parameters.enabled",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("includeWhen must be wrapped in ${...}"))
		})

		// Schema-aware validation catches forEach with non-iterable types at validation time
		It("should reject forEach with non-iterable expression", func() {
			obj.Spec.Schema = openchoreodevv1alpha1.ComponentTypeSchema{
				Parameters: &runtime.RawExtension{
					Raw: []byte(`{"replicas": "integer"}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:      "deployment",
					ForEach: "${parameters.replicas}",
					Var:     "item",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			// Schema-aware validation catches this error
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forEach expression must return list or map"))
		})
	})

	Context("Workload Resource Shape Validation", func() {
		It("should reject when no resource matches workloadType", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
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
			Expect(err.Error()).To(ContainSubstring("deployment"))
		})

		It("should reject when multiple resources match workloadType", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment1",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test1"}}`),
					},
				},
				{
					ID: "deployment2",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test2"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must have exactly one resource with kind matching workloadType"))
			Expect(err.Error()).To(ContainSubstring("found 2"))
		})

		It("should reject nil template in resource", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: nil,
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is required"))
		})

		It("should reject empty template in resource", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(``),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is required"))
		})

		It("should reject nil template in proxy workloadType", func() {
			obj.Spec.WorkloadType = "proxy"
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "gateway",
					Template: nil,
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template is required"))
		})

		It("should reject missing apiVersion in template", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
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

		It("should reject missing kind in template", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
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

		It("should reject missing metadata.name in template", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
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

		It("should allow workloadType=proxy without matching resource kind", func() {
			obj.Spec.WorkloadType = "proxy"
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "gateway",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should match workloadType case-insensitively", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "DEPLOYMENT", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("Embedded Traits Validation", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
		})

		It("should admit valid embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "app-data",
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"volumeName": "app-data", "mountPath": "${parameters.storage.mountPath}"}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit embedded traits with envOverrides", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "app-data",
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"volumeName": "app-data"}`),
					},
					EnvOverrides: &runtime.RawExtension{
						Raw: []byte(`{"size": "${envOverrides.storage.size}"}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject embedded trait with empty name", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "",
					InstanceName: "app-data",
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("name"))
			Expect(err.Error()).To(ContainSubstring("Required"))
		})

		It("should reject embedded trait with empty instanceName", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "",
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("instanceName"))
			Expect(err.Error()).To(ContainSubstring("Required"))
		})

		It("should reject duplicate instanceNames among embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "storage",
				},
				{
					Name:         "emptydir-volume",
					InstanceName: "storage",
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("instanceName"))
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})

		It("should allow multiple embedded traits with unique instanceNames", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "data-storage",
				},
				{
					Name:         "persistent-volume",
					InstanceName: "log-storage",
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("AllowedTraits Validation", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
		})

		It("should admit valid allowedTraits list", func() {
			obj.Spec.AllowedTraits = []string{"autoscaler", "rate-limiter"}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit empty allowedTraits (all traits allowed)", func() {
			obj.Spec.AllowedTraits = nil

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject empty string in allowedTraits", func() {
			obj.Spec.AllowedTraits = []string{"autoscaler", ""}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not be empty"))
		})

		It("should reject duplicate entries in allowedTraits", func() {
			obj.Spec.AllowedTraits = []string{"autoscaler", "rate-limiter", "autoscaler"}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})

		It("should reject allowedTraits that overlap with embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "app-data",
				},
			}
			obj.Spec.AllowedTraits = []string{"persistent-volume", "autoscaler"}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already embedded"))
		})

		It("should admit allowedTraits with no overlap with embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "app-data",
				},
			}
			obj.Spec.AllowedTraits = []string{"autoscaler", "rate-limiter"}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
