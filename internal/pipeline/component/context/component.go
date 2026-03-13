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

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// BuildComponentContext builds a CEL evaluation context for rendering component resources.
//
// The context includes:
//   - parameters: From Component.Spec.Parameters (pruned to schema.parameters) - access via ${parameters.*}
//   - environmentConfigs: From ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs (pruned to schema.environmentConfigs) - access via ${environmentConfigs.*}
//   - workload: Workload specification (image, resources, etc.) - access via ${workload.*}
//   - metadata: Structured naming and labeling information - access via ${metadata.*}
//   - dataplane: Data plane configuration - access via ${dataplane.*}
//   - configurations: Extracted configuration items from workload - access via ${configurations.*}
//
// Schema defaults are applied to both parameters and environmentConfigs sections.
func BuildComponentContext(input *ComponentContextInput) (*ComponentContext, error) {
	if err := validate.Struct(input); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	ctx := &ComponentContext{}

	// Process parameters and environmentConfigs separately
	parameters, envConfigs, err := processComponentParameters(input)
	if err != nil {
		return nil, err
	}
	ctx.Parameters = parameters
	ctx.EnvironmentConfigs = envConfigs

	// WorkloadData, Configurations, and Connections should be pre-computed by the caller
	ctx.Workload = input.WorkloadData
	ctx.Configurations = input.Configurations
	ctx.Connections = newConnectionsContextData(input.Connections)

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
	ctx.Environment = extractEnvironmentData(input.Environment, input.DataPlane, input.DefaultNotificationChannel)
	ctx.Gateway = ctx.Environment.Gateway

	return ctx, nil
}

// processComponentParameters processes component parameters and environmentConfigs separately,
// validates each against their respective schemas, and returns them as separate maps.
// Parameters come from Component.Spec.Parameters only.
// EnvironmentConfigs come from ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs only.
func processComponentParameters(input *ComponentContextInput) (map[string]any, map[string]any, error) {
	// Build both schema bundles
	parametersBundle, envConfigsBundle, err := BuildStructuralSchemas(&SchemaInput{
		ParametersSchema:         input.ComponentType.Spec.Parameters,
		EnvironmentConfigsSchema: input.ComponentType.Spec.EnvironmentConfigs,
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

	// Process environmentConfigs: ONLY from ReleaseBinding (no merging with Component)
	var envConfigs map[string]any
	if input.ReleaseBinding != nil && input.ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs != nil {
		envConfigs, err = extractParameters(input.ReleaseBinding.Spec.ComponentTypeEnvironmentConfigs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to extract environment configs: %w", err)
		}
	} else {
		envConfigs = make(map[string]any)
	}

	// Prune against schema, apply defaults, and validate
	if envConfigsBundle != nil {
		pruning.Prune(envConfigs, envConfigsBundle.Structural, false)
		envConfigs = schema.ApplyDefaults(envConfigs, envConfigsBundle.Structural)
		if err := schema.ValidateWithJSONSchema(envConfigs, envConfigsBundle.JSONSchema); err != nil {
			return nil, nil, fmt.Errorf("environmentConfigs validation failed: %w", err)
		}
	} else {
		// No environmentConfigs schema defined - discard all environmentConfigs
		envConfigs = make(map[string]any)
	}

	return parameters, envConfigs, nil
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
	data := DataPlaneData{}
	if dp.Spec.SecretStoreRef != nil {
		data.SecretStore = dp.Spec.SecretStoreRef.Name
	}
	if dp.Spec.ObservabilityPlaneRef != nil {
		data.ObservabilityPlaneRef = &ObservabilityPlaneRefData{
			Kind: string(dp.Spec.ObservabilityPlaneRef.Kind),
			Name: dp.Spec.ObservabilityPlaneRef.Name,
		}
	}
	data.Gateway = toGatewayData(&dp.Spec.Gateway)
	return data
}

// toGatewayData converts a v1alpha1.GatewaySpec to a GatewayData for template context.
func toGatewayData(gw *v1alpha1.GatewaySpec) *GatewayData {
	if gw == nil {
		return nil
	}
	data := &GatewayData{}
	if gw.Ingress != nil {
		data.Ingress = toGatewayNetworkData(gw.Ingress)
	}
	if gw.Egress != nil {
		data.Egress = toGatewayNetworkData(gw.Egress)
	}
	if data.Ingress == nil && data.Egress == nil {
		return nil
	}
	return data
}

// toGatewayNetworkData converts a v1alpha1.GatewayNetworkSpec to a GatewayNetworkData.
func toGatewayNetworkData(t *v1alpha1.GatewayNetworkSpec) *GatewayNetworkData {
	if t == nil {
		return nil
	}
	data := &GatewayNetworkData{}
	if t.External != nil {
		data.External = toGatewayEndpointData(t.External)
	}
	if t.Internal != nil {
		data.Internal = toGatewayEndpointData(t.Internal)
	}
	return data
}

// toGatewayEndpointData converts a v1alpha1.GatewayEndpointSpec to a GatewayEndpointData.
func toGatewayEndpointData(e *v1alpha1.GatewayEndpointSpec) *GatewayEndpointData {
	if e == nil {
		return nil
	}
	data := &GatewayEndpointData{
		Name:      e.Name,
		Namespace: e.Namespace,
	}
	if e.HTTP != nil {
		data.HTTP = &GatewayListenerData{
			ListenerName: e.HTTP.ListenerName,
			Port:         e.HTTP.Port,
			Host:         e.HTTP.Host,
		}
	}
	if e.HTTPS != nil {
		data.HTTPS = &GatewayListenerData{
			ListenerName: e.HTTPS.ListenerName,
			Port:         e.HTTPS.Port,
			Host:         e.HTTPS.Host,
		}
	}
	if e.TLS != nil {
		data.TLS = &GatewayListenerData{
			ListenerName: e.TLS.ListenerName,
			Port:         e.TLS.Port,
			Host:         e.TLS.Host,
		}
	}
	return data
}

// extractEnvironmentData extracts EnvironmentData from Environment and DataPlane resources.
// Gateway configuration is merged at each dimension: ingress/egress and external/internal.
// Environment-level values take precedence; missing values fall back to the DataPlane level.
func extractEnvironmentData(env *v1alpha1.Environment, dp *v1alpha1.DataPlane, defaultNotificationChannel string) EnvironmentData {
	return EnvironmentData{
		Gateway:                    mergeGatewayData(&env.Spec.Gateway, &dp.Spec.Gateway),
		DefaultNotificationChannel: defaultNotificationChannel,
	}
}

// mergeGatewayData merges environment-level and dataplane-level gateway specs.
// Ingress and egress are merged independently, so specifying only one at the environment
// level still falls back to the dataplane value for the other.
func mergeGatewayData(envGW, dpGW *v1alpha1.GatewaySpec) *GatewayData {
	var envIngress, dpIngress *v1alpha1.GatewayNetworkSpec
	var envEgress, dpEgress *v1alpha1.GatewayNetworkSpec

	if envGW != nil {
		envIngress = envGW.Ingress
		envEgress = envGW.Egress
	}
	if dpGW != nil {
		dpIngress = dpGW.Ingress
		dpEgress = dpGW.Egress
	}

	data := &GatewayData{
		Ingress: mergeGatewayNetworkData(envIngress, dpIngress),
		Egress:  mergeGatewayNetworkData(envEgress, dpEgress),
	}
	if data.Ingress == nil && data.Egress == nil {
		return nil
	}
	return data
}

// mergeGatewayNetworkData merges environment-level and dataplane-level gateway network specs.
// External and internal endpoints are merged independently, preferring environment values
// and falling back to dataplane values for any unspecified endpoint.
func mergeGatewayNetworkData(envNetwork, dpNetwork *v1alpha1.GatewayNetworkSpec) *GatewayNetworkData {
	if envNetwork == nil && dpNetwork == nil {
		return nil
	}

	var envExternal, dpExternal *v1alpha1.GatewayEndpointSpec
	var envInternal, dpInternal *v1alpha1.GatewayEndpointSpec

	if envNetwork != nil {
		envExternal = envNetwork.External
		envInternal = envNetwork.Internal
	}
	if dpNetwork != nil {
		dpExternal = dpNetwork.External
		dpInternal = dpNetwork.Internal
	}

	external := envExternal
	if external == nil {
		external = dpExternal
	}
	internal := envInternal
	if internal == nil {
		internal = dpInternal
	}

	data := &GatewayNetworkData{
		External: toGatewayEndpointData(external),
		Internal: toGatewayEndpointData(internal),
	}
	if data.External == nil && data.Internal == nil {
		return nil
	}
	return data
}

// ExtractWorkloadData extracts relevant workload information for the rendering context.
// This function is exported so callers can pre-compute workload data once and share
// it across multiple context builds (ComponentContext and TraitContexts).
func ExtractWorkloadData(workload *v1alpha1.Workload) WorkloadData {
	data := WorkloadData{
		Endpoints: make(map[string]EndpointData),
	}

	if workload == nil {
		return data
	}

	data.Container = ContainerData{
		Image:   workload.Spec.Container.Image,
		Command: workload.Spec.Container.Command,
		Args:    workload.Spec.Container.Args,
	}

	for name, endpoint := range workload.Spec.Endpoints {
		targetPort := endpoint.TargetPort
		if targetPort == 0 {
			targetPort = endpoint.Port
		}

		// Build visibility array: always include "project", then append additional unique visibilities.
		visibilitySet := map[string]struct{}{string(v1alpha1.EndpointVisibilityProject): {}}
		visibility := []string{string(v1alpha1.EndpointVisibilityProject)}
		for _, v := range endpoint.Visibility {
			vs := string(v)
			if _, exists := visibilitySet[vs]; !exists {
				visibilitySet[vs] = struct{}{}
				visibility = append(visibility, vs)
			}
		}

		epData := EndpointData{
			DisplayName: endpoint.DisplayName,
			Port:        endpoint.Port,
			TargetPort:  targetPort,
			Type:        string(endpoint.Type),
			BasePath:    endpoint.BasePath,
			Visibility:  visibility,
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

// BuildStructuralSchemas builds separate structural schemas for parameters and environmentConfigs.
//
// Returns (parametersBundle, envConfigsBundle, error). Either bundle's schemas can be nil if not provided.
func BuildStructuralSchemas(input *SchemaInput) (*SchemaBundle, *SchemaBundle, error) {
	// Build parameters schema bundle
	var parametersBundle *SchemaBundle
	structural, jsonSchema, err := schema.ResolveSectionToBundle(input.ParametersSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create parameters schema: %w", err)
	}
	if structural != nil {
		parametersBundle = &SchemaBundle{
			Structural: structural,
			JSONSchema: jsonSchema,
		}
	}

	// Build environmentConfigs schema bundle
	var envConfigsBundle *SchemaBundle
	structural, jsonSchema, err = schema.ResolveSectionToBundle(input.EnvironmentConfigsSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create environmentConfigs schema: %w", err)
	}
	if structural != nil {
		envConfigsBundle = &SchemaBundle{
			Structural: structural,
			JSONSchema: jsonSchema,
		}
	}

	return parametersBundle, envConfigsBundle, nil
}
