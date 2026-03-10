// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
)

// OpenAPIV3ToStructural resolves $refs and converts an openAPIV3Schema to a
// Kubernetes structural schema. Used by webhooks and pipeline for validation and defaulting.
func OpenAPIV3ToStructural(rawSchema map[string]any) (*apiextschema.Structural, error) {
	internalSchema, err := openAPIV3ToInternal(rawSchema)
	if err != nil {
		return nil, err
	}

	structural, err := apiextschema.NewStructural(internalSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to build structural schema: %w", err)
	}
	return structural, nil
}

// OpenAPIV3ToJSONSchema resolves $refs and converts an openAPIV3Schema to
// v1 JSONSchemaProps for API responses. Note: arbitrary vendor extensions (x-*)
// are NOT preserved because extv1.JSONSchemaProps does not support them.
// Use OpenAPIV3ToResolvedSchema for API responses that need vendor extensions.
func OpenAPIV3ToJSONSchema(rawSchema map[string]any) (*extv1.JSONSchemaProps, error) {
	resolved, err := ResolveRefs(rawSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve $ref: %w", err)
	}

	v1Schema, err := mapToV1JSONSchema(resolved)
	if err != nil {
		return nil, err
	}

	sortRequiredFields(v1Schema)
	return v1Schema, nil
}

// OpenAPIV3ToResolvedSchema resolves $refs and removes $defs, returning the resolved
// schema as a raw map. Unlike OpenAPIV3ToJSONSchema, this preserves all vendor
// extensions (x-*) since it does not convert through extv1.JSONSchemaProps.
// Use this for API responses where vendor extensions must be returned to the frontend.
func OpenAPIV3ToResolvedSchema(rawSchema map[string]any) (map[string]any, error) {
	resolved, err := ResolveRefs(rawSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve $ref: %w", err)
	}
	return resolved, nil
}

// OpenAPIV3ToStructuralAndJSONSchema returns both structural and JSON schema formats
// from an openAPIV3Schema in one pass. Used by pipeline context (BuildStructuralSchemas).
func OpenAPIV3ToStructuralAndJSONSchema(rawSchema map[string]any) (*apiextschema.Structural, *extv1.JSONSchemaProps, error) {
	resolved, err := ResolveRefs(rawSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve $ref: %w", err)
	}

	// For structural: strip vendor extensions (K8s rejects x-* keys)
	stripped := stripVendorExtensions(resolved)
	internalSchema, err := mapToInternalJSONSchema(stripped)
	if err != nil {
		return nil, nil, err
	}

	structural, err := apiextschema.NewStructural(internalSchema)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build structural schema: %w", err)
	}

	// For JSON schema: preserve vendor extensions
	v1Schema, err := mapToV1JSONSchema(resolved)
	if err != nil {
		return nil, nil, err
	}

	sortRequiredFields(v1Schema)
	return structural, v1Schema, nil
}

// openAPIV3ToInternal resolves refs, strips vendor extensions, and converts to internal type.
func openAPIV3ToInternal(rawSchema map[string]any) (*apiext.JSONSchemaProps, error) {
	resolved, err := ResolveRefs(rawSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve $ref: %w", err)
	}

	stripped := stripVendorExtensions(resolved)

	return mapToInternalJSONSchema(stripped)
}

// mapToInternalJSONSchema marshals a map to JSON and unmarshals into internal JSONSchemaProps.
func mapToInternalJSONSchema(schema map[string]any) (*apiext.JSONSchemaProps, error) {
	// Marshal to v1 first (has proper JSON tags), then convert to internal
	v1Schema, err := mapToV1JSONSchema(schema)
	if err != nil {
		return nil, err
	}

	internalSchema := new(apiext.JSONSchemaProps)
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(v1Schema, internalSchema, nil); err != nil {
		return nil, fmt.Errorf("failed to convert schema to internal type: %w", err)
	}
	return internalSchema, nil
}

// mapToV1JSONSchema marshals a map to JSON and unmarshals into v1 JSONSchemaProps.
func mapToV1JSONSchema(schema map[string]any) (*extv1.JSONSchemaProps, error) {
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %w", err)
	}

	v1Schema := new(extv1.JSONSchemaProps)
	if err := json.Unmarshal(data, v1Schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schema to JSONSchemaProps: %w", err)
	}
	return v1Schema, nil
}

// stripVendorExtensions recursively removes vendor extension keys (x-*) from a schema tree,
// but preserves x-kubernetes-* keys which are supported by Kubernetes structural schemas.
func stripVendorExtensions(schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}

	result := make(map[string]any, len(schema))
	for k, v := range schema {
		if len(k) > 2 && k[0] == 'x' && k[1] == '-' && !strings.HasPrefix(k, "x-kubernetes-") {
			continue
		}
		result[k] = stripVendorExtensionsValue(v)
	}
	return result
}

// stripVendorExtensionsValue recursively strips vendor extensions from any value.
func stripVendorExtensionsValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return stripVendorExtensions(val)
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = stripVendorExtensionsValue(item)
		}
		return result
	default:
		return v
	}
}
