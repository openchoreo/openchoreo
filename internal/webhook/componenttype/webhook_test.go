// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componenttype

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	workloadTypeDeployment = "deployment"
	workloadTypeProxy      = "proxy"
)

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
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
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

		It("should admit valid ComponentType with parameters and environmentConfigs", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
				},
			}
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"image":{"type":"string"}},"required":["image"]}`),
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
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":2}}}`),
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

		It("should reject invalid JSON in spec.parameters.openAPIV3Schema $types", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{

				OpenAPIV3Schema: &runtime.RawExtension{

					Raw: []byte(`{"$types": {malformed json}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "expected NewInvalid wrap so kube returns 422")
		})

		It("should reject invalid JSON in spec.parameters.openAPIV3Schema", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{

				OpenAPIV3Schema: &runtime.RawExtension{

					Raw: []byte(`{malformed`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject invalid JSON in spec.environmentConfigs.openAPIV3Schema", func() {
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{

				OpenAPIV3Schema: &runtime.RawExtension{

					Raw: []byte(`not valid yaml`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse environmentConfigs schema"))
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

		It("should reject invalid OpenAPI v3 schema in parameters", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":"not-an-object"}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject circular $ref in parameters", func() {
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"A":{"$ref":"#/$defs/B"},"B":{"$ref":"#/$defs/A"}},"properties":{"val":{"$ref":"#/$defs/A"}}}`),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
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
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"}},"required":["replicas"]}`),
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
			obj.Spec.WorkloadType = workloadTypeProxy
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

		It("should reject additional workload resource kind that does not match workloadType", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
				{
					ID: "statefulset",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "StatefulSet", "metadata": {"name": "extra-sts"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("does not match the declared workloadType"))
			Expect(err.Error()).To(ContainSubstring("StatefulSet"))
		})

		It("should reject multiple non-matching workload resource kinds", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
				{
					ID: "job",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "batch/v1", "kind": "Job", "metadata": {"name": "extra-job"}}`),
					},
				},
				{
					ID: "cronjob",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "batch/v1", "kind": "CronJob", "metadata": {"name": "extra-cron"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(MatchRegexp(`"Job"`))
			Expect(err.Error()).To(ContainSubstring("CronJob"))
		})

		It("should admit Deployment with non-workload resources like Service", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
				{
					ID: "service",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "v1", "kind": "Service", "metadata": {"name": "test-svc"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject workload resource in proxy ComponentType", func() {
			obj.Spec.WorkloadType = workloadTypeProxy
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "gateway",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway", "metadata": {"name": "test"}}`),
					},
				},
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "extra"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("proxy ComponentType must not contain workload resources"))
			Expect(err.Error()).To(ContainSubstring("Deployment"))
		})

		It("should allow workloadType=proxy without matching resource kind", func() {
			obj.Spec.WorkloadType = workloadTypeProxy
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

		It("should reject template expression in kind field", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "${parameters.kind}", "metadata": {"name": "test"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("kind must be a literal value"))
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

		It("should allow template with metadata.namespace set to ${metadata.namespace}", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test", "namespace": "${metadata.namespace}"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject template with hardcoded metadata.namespace", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test", "namespace": "my-namespace"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("${metadata.namespace}"))
		})

		It("should allow template with whitespace inside ${metadata.namespace}", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test", "namespace": "${ metadata.namespace }"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject template with different CEL expression in metadata.namespace", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test", "namespace": "${parameters.namespace}"}}`),
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("${metadata.namespace}"))
		})

		It("should allow template without metadata.namespace", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: "deployment",
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": {"name": "test"}}`),
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

		It("should admit embedded traits with environmentConfigs", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Name:         "persistent-volume",
					InstanceName: "app-data",
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"volumeName": "app-data"}`),
					},
					EnvironmentConfigs: &runtime.RawExtension{
						Raw: []byte(`{"size": "${environmentConfigs.storage.size}"}`),
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

	Context("Validation Rules CEL Validation", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
		})

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
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("OpenAPIV3Schema Support", func() {
		It("should admit valid ComponentType with openAPIV3Schema parameters", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1},"image":{"type":"string"}}}`),
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

		It("should admit valid ComponentType with openAPIV3Schema parameters and environmentConfigs", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"name":{"type":"string","default":"app"}}}`),
				},
			}
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
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

		It("should admit openAPIV3Schema with $defs and $ref", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"Port":{"type":"integer","minimum":1,"maximum":65535,"default":8080}},"properties":{"port":{"$ref":"#/$defs/Port"}}}`),
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

		It("should admit openAPIV3Schema with vendor extensions (x-*)", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"url":{"type":"string","x-openchoreo-backstage-portal":{"ui:field":"RepoUrlPicker"}}}}`),
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

		It("should reject openAPIV3Schema with circular $ref", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"A":{"$ref":"#/$defs/B"},"B":{"$ref":"#/$defs/A"}},"properties":{"val":{"$ref":"#/$defs/A"}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should reject malformed openAPIV3Schema JSON", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{not valid json`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse parameters schema"))
		})

		It("should validate CEL expressions against openAPIV3Schema types", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1},"enabled":{"type":"boolean","default":true}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:          "deployment",
					IncludeWhen: "${parameters.enabled}",
					Template:    deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject forEach with non-iterable openAPIV3Schema type", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer"}}}`),
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
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("forEach expression must return list or map"))
		})

		It("should validate boolean validation rules with openAPIV3Schema", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":1}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: deploymentTemplateWithCEL("${parameters.replicas}"),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.replicas > 0}", Message: "replicas must be positive"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject non-boolean validation rule with openAPIV3Schema", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"name":{"type":"string","default":"app"}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}
			obj.Spec.Validations = []openchoreodevv1alpha1.ValidationRule{
				{Rule: "${parameters.name}", Message: "returns string not bool"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("rule must return boolean"))
		})

		It("should admit valid update with openAPIV3Schema", func() {
			oldObj.Spec.WorkloadType = workloadTypeDeployment
			oldObj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"replicas":{"type":"integer","default":2}}}`),
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

		It("should admit openAPIV3Schema with nested objects and required fields", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Parameters = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","properties":{"database":{"type":"object","properties":{"host":{"type":"string"},"port":{"type":"integer","default":5432}},"required":["host"]}}}`),
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

		It("should reject openAPIV3Schema in environmentConfigs with circular ref", func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.EnvironmentConfigs = &openchoreodevv1alpha1.SchemaSection{
				OpenAPIV3Schema: &runtime.RawExtension{
					Raw: []byte(`{"type":"object","$defs":{"X":{"$ref":"#/$defs/Y"},"Y":{"$ref":"#/$defs/X"}},"properties":{"val":{"$ref":"#/$defs/X"}}}`),
				},
			}
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID:       "deployment",
					Template: validDeploymentTemplate(),
				},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse environmentConfigs schema"))
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
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "rate-limiter"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit allowedTraits with ClusterTrait kind", func() {
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindClusterTrait, Name: "rate-limiter"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should admit empty allowedTraits (all traits allowed)", func() {
			obj.Spec.AllowedTraits = nil

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject empty string in allowedTraits", func() {
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: ""},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not be empty"))
		})

		It("should reject duplicate entries in allowedTraits", func() {
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "rate-limiter"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})

		It("should allow same name with different kinds in allowedTraits", func() {
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindClusterTrait, Name: "autoscaler"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject allowedTraits that overlap with embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Kind:         openchoreodevv1alpha1.TraitRefKindTrait,
					Name:         "persistent-volume",
					InstanceName: "app-data",
				},
			}
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "persistent-volume"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("already embedded"))
		})

		It("should admit allowedTraits with no overlap with embedded traits", func() {
			obj.Spec.Traits = []openchoreodevv1alpha1.ComponentTypeTrait{
				{
					Kind:         openchoreodevv1alpha1.TraitRefKindTrait,
					Name:         "persistent-volume",
					InstanceName: "app-data",
				},
			}
			obj.Spec.AllowedTraits = []openchoreodevv1alpha1.TraitRef{
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "autoscaler"},
				{Kind: openchoreodevv1alpha1.TraitRefKindTrait, Name: "rate-limiter"},
			}

			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	Context("allowedWorkflows validation", func() {
		BeforeEach(func() {
			obj.Spec.WorkloadType = workloadTypeDeployment
			obj.Spec.Resources = []openchoreodevv1alpha1.ResourceTemplate{
				{
					ID: workloadTypeDeployment,
					Template: &runtime.RawExtension{
						Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":"test"}}`),
					},
				},
			}
		})

		It("should admit valid allowedWorkflows", func() {
			obj.Spec.AllowedWorkflows = []openchoreodevv1alpha1.WorkflowRef{
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "build"},
				{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "deploy"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should reject empty workflow name in allowedWorkflows", func() {
			obj.Spec.AllowedWorkflows = []openchoreodevv1alpha1.WorkflowRef{
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: ""},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("must not be empty"))
		})

		It("should reject duplicate workflow by kind:name", func() {
			obj.Spec.AllowedWorkflows = []openchoreodevv1alpha1.WorkflowRef{
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "build"},
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "build"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})

		It("should allow same name with different kinds in allowedWorkflows", func() {
			obj.Spec.AllowedWorkflows = []openchoreodevv1alpha1.WorkflowRef{
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "build"},
				{Kind: openchoreodevv1alpha1.WorkflowRefKindClusterWorkflow, Name: "build"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should default empty Kind to Workflow when checking duplicates", func() {
			obj.Spec.AllowedWorkflows = []openchoreodevv1alpha1.WorkflowRef{
				{Name: "build"},
				{Kind: openchoreodevv1alpha1.WorkflowRefKindWorkflow, Name: "build"},
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Duplicate"))
		})
	})

	Context("ValidateDelete", func() {
		It("should admit deletion of a valid ComponentType", func() {
			_, err := validator.ValidateDelete(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return an error when given a non-ComponentType object", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterComponentType{}
			_, err := validator.ValidateDelete(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ComponentType object"))
		})
	})

	Context("ValidateCreate and ValidateUpdate wrong type errors", func() {
		It("should return an error when ValidateCreate receives a non-ComponentType object", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterComponentType{}
			_, err := validator.ValidateCreate(ctx, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ComponentType object"))
		})

		It("should return an error when ValidateUpdate oldObj is not a ComponentType", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterComponentType{}
			_, err := validator.ValidateUpdate(ctx, wrongObj, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ComponentType object for the oldObj"))
		})

		It("should return an error when ValidateUpdate newObj is not a ComponentType", func() {
			wrongObj := &openchoreodevv1alpha1.ClusterComponentType{}
			_, err := validator.ValidateUpdate(ctx, oldObj, wrongObj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("expected a ComponentType object for the newObj"))
		})
	})
})
