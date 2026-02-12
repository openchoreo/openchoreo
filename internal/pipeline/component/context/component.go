// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"encoding/json"
	"fmt"
	"maps"

	"github.com/go-playground/validator/v10"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/pruning"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// BuildComponentContext builds a CEL evaluation context for rendering component resources.
//
// The context includes:
//   - parameters: From Component.Spec.Parameters (pruned to schema.parameters) - access via ${parameters.*}
//   - envOverrides: From ReleaseBinding.Spec.ComponentTypeEnvOverrides (pruned to schema.envOverrides) - access via ${envOverrides.*}
//   - workload: Workload specification (image, resources, etc.) - access via ${workload.*}
//   - metadata: Structured naming and labeling information - access via ${metadata.*}
//   - dataplane: Data plane configuration - access via ${dataplane.*}
//   - configurations: Extracted configuration items from workload - access via ${configurations.*}
//
// Schema defaults are applied to both parameters and envOverrides sections.
func BuildComponentContext(input *ComponentContextInput) (*ComponentContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	ctx := &ComponentContext{}

	// Process parameters and envOverrides separately
	parameters, envOverrides, err := processComponentParameters(input)
	if err != nil {
		return nil, err
	}
	ctx.Parameters = parameters
	ctx.EnvOverrides = envOverrides

	// WorkloadData and Configurations should be pre-computed by the caller
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations

	// Ensure metadata maps are always initialized
	ctx.Metadata = input.Metadata
	if ctx.Metadata.Labels == nil {
		ctx.Metadata.Labels = make(map[string]string)
	}
	if ctx.Metadata.Annotations == nil {
		ctx.Metadata.Annotations = make(map[string]string)
	}
	if ctx.Metadata.PodSelectors == nil {
		ctx.Metadata.PodSelectors = make(map[string]string)
	}

	ctx.DataPlane = extractDataPlaneData(input.DataPlane)
	ctx.Environment = extractEnvironmentData(input.Environment, input.DataPlane)

	return ctx, nil
}

// processComponentParameters processes component parameters and envOverrides separately,
// validates each against their respective schemas, and returns them as separate maps.
// Parameters come from Component.Spec.Parameters only.
// EnvOverrides come from ReleaseBinding.Spec.ComponentTypeEnvOverrides only.
func processComponentParameters(input *ComponentContextInput) (map[string]any, map[string]any, error) {
	// Build both schema bundles in one call to share types unmarshaling
	parametersBundle, envOverridesBundle, err := BuildStructuralSchemas(&SchemaInput{
		Types:              input.ComponentType.Spec.Schema.Types,
		ParametersSchema:   input.ComponentType.Spec.Schema.Parameters,
		EnvOverridesSchema: input.ComponentType.Spec.Schema.EnvOverrides,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build schemas: %w", err)
	}

	// Extract component parameters (for parameters section only)
	componentParams, err := extractParameters(input.Component.Spec.Parameters)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract component parameters: %w", err)
	}

	// Process parameters: prune to parameters schema, apply defaults, validate
	var parameters map[string]any
	if parametersBundle != nil {
		parameters = make(map[string]any, len(componentParams))
		maps.Copy(parameters, componentParams)
		pruning.Prune(parameters, parametersBundle.Structural, false)
		parameters = schema.ApplyDefaults(parameters, parametersBundle.Structural)
		if err := schema.ValidateWithJSONSchema(parameters, parametersBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("parameters validation failed: %w", err)
		}
	} else {
		// No parameters schema defined - discard all parameters
		parameters = make(map[string]any)
	}

	// Process envOverrides: ONLY from ReleaseBinding (no merging with Component)
	var envOverrides map[string]any
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.ComponentTypeEnvOverrides != nil {
		envOverrides, err = extractParameters(input.ReleaseBinding.Spec.ComponentTypeEnvOverrides)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract environment overrides: %w", err)
		}
	} else {
		envOverrides = make(map[string]any)
	}

	// Prune against schema, apply defaults, and validate
	if envOverridesBundle != nil {
		pruning.Prune(envOverrides, envOverridesBundle.Structural, false)
		envOverrides = schema.ApplyDefaults(envOverrides, envOverridesBundle.Structural)
		if err := schema.ValidateWithJSONSchema(envOverrides, envOverridesBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("envOverrides validation failed: %w", err)
		}
	} else {
		// No envOverrides schema defined - discard all envOverrides
		envOverrides = make(map[string]any)
	}

	return parameters, envOverrides, nil
}

// ToMap converts the ComponentContext to map[string]any for CEL evaluation.
// All fields including configurations are converted to nested maps via JSON marshaling.
// This allows consistent CEL access without requiring ext.NativeTypes() registration.
func (c *ComponentContext) ToMap() map[string]any {
	result, err := structToMap(c)
	if err != nil {
		// This should never happen with well-formed ComponentContext
		return make(map[string]any)
	}
	return result
}

// extractParameters converts a runtime.RawExtension to a map for CEL evaluation.
//
// runtime.RawExtension is Kubernetes' way of storing arbitrary JSON in a typed field.
// This function unmarshals the raw JSON bytes into a map that can be:
//   - Merged with other parameter sources
//   - Used as CEL evaluation context
//   - Validated against schemas
//
// Returns an empty map if the extension is nil or empty, rather than an error,
// since absent parameters are valid (defaults will be applied by schema).
func extractParameters(raw *runtime.RawExtension) (map[string]any, error) {
	if raw == nil || raw.Raw == nil {
		return make(map[string]any), nil
	}

	var params map[string]any
	if err := json.Unmarshal(raw.Raw, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return params, nil
}

// extractDataPlaneData extracts DataPlaneData from a DataPlane resource.
func extractDataPlaneData(dp *v1alpha1.DataPlane) DataPlaneData {
	data := DataPlaneData{
		PublicVirtualHost: dp.Spec.Gateway.PublicVirtualHost,
	}
	if dp.Spec.SecretStoreRef != nil {
		data.SecretStore = dp.Spec.SecretStoreRef.Name
	}
	if dp.Spec.Gateway.OrganizationVirtualHost != "" {
		data.OrganizationVirtualHost = dp.Spec.Gateway.OrganizationVirtualHost
	}
	if dp.Spec.ObservabilityPlaneRef != nil {
		data.ObservabilityPlaneRef = &ObservabilityPlaneRefData{
			Kind: string(dp.Spec.ObservabilityPlaneRef.Kind),
			Name: dp.Spec.ObservabilityPlaneRef.Name,
		}
	}
	return data
}

// extractEnvironmentData extracts EnvironmentData from Environment and DataPlane resources.
// If the Environment has gateway configuration, it uses those values.
// Otherwise, it falls back to the DataPlane gateway configuration.
// Gateway name and namespace default to "gateway-default" and "openchoreo-data-plane" if not set.
func extractEnvironmentData(env *v1alpha1.Environment, dp *v1alpha1.DataPlane) EnvironmentData {
	// If environment has gateway configuration, use it
	if env.Spec.Gateway.PublicVirtualHost != "" {
		return EnvironmentData{
			PublicVirtualHost:       env.Spec.Gateway.PublicVirtualHost,
			OrganizationVirtualHost: env.Spec.Gateway.OrganizationVirtualHost,
		}
	}

	// Fallback to DataPlane gateway configuration
	return EnvironmentData{
		PublicVirtualHost:       dp.Spec.Gateway.PublicVirtualHost,
		OrganizationVirtualHost: dp.Spec.Gateway.OrganizationVirtualHost,
	}
}

// ExtractWorkloadData extracts relevant workload information for the rendering context.
// This function is exported so callers can pre-compute workload data once and share
// it across multiple context builds (ComponentContext and TraitContexts).
func ExtractWorkloadData(workload *v1alpha1.Workload) WorkloadData {
	data := WorkloadData{
		Containers: make(map[string]ContainerData),
		Endpoints:  make(map[string]EndpointData),
	}

	if workload == nil {
		return data
	}

	for name, container := range workload.Spec.Containers {
		data.Containers[name] = ContainerData{
			Image:   container.Image,
			Command: container.Command,
			Args:    container.Args,
		}
	}

	for name, endpoint := range workload.Spec.Endpoints {
		epData := EndpointData{
			Type: string(endpoint.Type),
			Port: endpoint.Port,
		}
		if endpoint.Schema != nil {
			epData.Schema = &SchemaData{
				Type:    endpoint.Schema.Type,
				Content: endpoint.Schema.Content,
			}
		}
		data.Endpoints[name] = epData
	}

	return data
}

// structToMap converts typed Go structs to map[string]any for CEL evaluation.
//
// CEL expressions can only access maps and primitive types, not arbitrary Go structs.
// This function uses JSON marshaling as a conversion mechanism:
func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// SchemaBundle holds both structural and JSON schemas for validation workflows.
// The structural schema is used for pruning and defaulting, while the JSON schema
// is used for validation.
type SchemaBundle struct {
	Structural *apiextschema.Structural
	JSONSchema *extv1.JSONSchemaProps
}

// BuildStructuralSchemas builds separate structural schemas for parameters and envOverrides
// while unmarshaling types only once.
//
// Returns (parametersBundle, envOverridesBundle, error). Either bundle's schemas can be nil if not provided.
func BuildStructuralSchemas(input *SchemaInput) (*SchemaBundle, *SchemaBundle, error) {
	// Extract types from RawExtension once
	var types map[string]any
	if input.Types != nil {
		if err := yaml.Unmarshal(input.Types.Raw, &types); err != nil {
			return nil, nil, fmt.Errorf("failed to extract types: %w", err)
		}
	}

	// Build parameters schema bundle
	var parametersBundle *SchemaBundle
	if input.ParametersSchema != nil {
		params, err := extractParameters(input.ParametersSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract parameters schema: %w", err)
		}

		def := schema.Definition{
			Types:   types,
			Schemas: []map[string]any{params},
		}

		structural, jsonSchema, err := schema.ToStructuralAndJSONSchema(def)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create parameters schema: %w", err)
		}
		parametersBundle = &SchemaBundle{
			Structural: structural,
			JSONSchema: jsonSchema,
		}
	}

	// Build envOverrides schema bundle
	var envOverridesBundle *SchemaBundle
	if input.EnvOverridesSchema != nil {
		envOverrides, err := extractParameters(input.EnvOverridesSchema)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract envOverrides schema: %w", err)
		}

		def := schema.Definition{
			Types:   types,
			Schemas: []map[string]any{envOverrides},
		}

		structural, jsonSchema, err := schema.ToStructuralAndJSONSchema(def)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create envOverrides schema: %w", err)
		}
		envOverridesBundle = &SchemaBundle{
			Structural: structural,
			JSONSchema: jsonSchema,
		}
	}

	return parametersBundle, envOverridesBundle, nil
}
