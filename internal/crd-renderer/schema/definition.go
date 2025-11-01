// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"fmt"
	"sort"

	apiext "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"

	"github.com/openchoreo/openchoreo/internal/crd-renderer/schemaextractor"
	"github.com/openchoreo/openchoreo/internal/crd-renderer/util"
)

// Definition represents a schematized object assembled from one or more field maps.
type Definition struct {
	Types   map[string]any
	Schemas []map[string]any
}

// ToJSONSchema converts the definition into an OpenAPI-compatible JSON schema.
func ToJSONSchema(def Definition) (*extv1.JSONSchemaProps, error) {
	merged := mergeFieldMaps(def.Schemas)
	if len(merged) == 0 {
		return &extv1.JSONSchemaProps{
			Type:       "object",
			Properties: map[string]extv1.JSONSchemaProps{},
		}, nil
	}

	jsonSchema, err := schemaextractor.ExtractSchema(merged, def.Types)
	if err != nil {
		return nil, fmt.Errorf("failed to convert schema to OpenAPI: %w", err)
	}

	sortRequiredFields(jsonSchema)
	return jsonSchema, nil
}

// ToStructural converts the definition into a structural schema that can be used for Kubernetes defaulting.
func ToStructural(def Definition) (*apiextschema.Structural, error) {
	jsonSchemaV1, err := ToJSONSchema(def)
	if err != nil {
		return nil, err
	}

	internal := new(apiext.JSONSchemaProps)
	if err := extv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(jsonSchemaV1, internal, nil); err != nil {
		return nil, fmt.Errorf("failed to convert schema: %w", err)
	}

	structural, err := apiextschema.NewStructural(internal)
	if err != nil {
		return nil, fmt.Errorf("failed to build structural schema: %w", err)
	}
	return structural, nil
}

// ApplyDefaults runs the Kubernetes defaulting algorithm against the provided map.
func ApplyDefaults(target map[string]any, structural *apiextschema.Structural) map[string]any {
	if structural == nil {
		return target
	}
	if target == nil {
		target = map[string]any{}
	}
	defaulting.Default(target, structural)
	return target
}

// mergeFieldMaps collapses the different field buckets (parameters, env overrides, addon inputs)
// into the single document expected by the schema extractor. ComponentTypeDefinition templates
// are authored this way so we preserve the same merge semantics they were written for.
func mergeFieldMaps(maps []map[string]any) map[string]any {
	result := map[string]any{}
	for _, fields := range maps {
		mergeInto(result, fields)
	}
	return result
}

func mergeInto(dst map[string]any, src map[string]any) {
	if src == nil {
		return
	}
	if dst == nil {
		// should not happen, but guard anyway
		return
	}
	for k, v := range src {
		if vMap, ok := v.(map[string]any); ok {
			existing, ok := dst[k].(map[string]any)
			if !ok {
				dst[k] = util.DeepCopy(vMap)
				continue
			}
			mergeInto(existing, vMap)
			continue
		}
		dst[k] = util.DeepCopy(v)
	}
}

func sortRequiredFields(schema *extv1.JSONSchemaProps) {
	if schema == nil {
		return
	}
	if len(schema.Required) > 0 {
		// Keep output deterministic for CLI/UI generators and to minimize diffs when definitions change.
		sort.Strings(schema.Required)
	}
	if schema.Properties != nil {
		for key := range schema.Properties {
			prop := schema.Properties[key]
			sortRequiredFields(&prop)
			schema.Properties[key] = prop
		}
	}
	if schema.Items != nil && schema.Items.Schema != nil {
		sortRequiredFields(schema.Items.Schema)
	}
	if schema.AdditionalProperties != nil && schema.AdditionalProperties.Schema != nil {
		sortRequiredFields(schema.AdditionalProperties.Schema)
	}
}
