// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package typed

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// FromEntry converts a ResourceEntry to a typed v1alpha1 object using runtime.DefaultUnstructuredConverter
func FromEntry[T any](entry *index.ResourceEntry) (*T, error) {
	if entry == nil {
		return nil, fmt.Errorf("resource entry is nil")
	}
	if entry.Resource == nil {
		return nil, fmt.Errorf("resource is nil")
	}

	var obj T
	err := runtime.DefaultUnstructuredConverter.FromUnstructured(entry.Resource.Object, &obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert unstructured to typed object: %w", err)
	}

	return &obj, nil
}

// rawExtensionToMap converts a runtime.RawExtension to map[string]interface{} for template processing
func rawExtensionToMap(raw *runtime.RawExtension) map[string]interface{} {
	if raw == nil || raw.Raw == nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw.Raw, &result); err != nil {
		return nil
	}

	return result
}

// apiSliceToInterface converts a slice of API structs into a []interface{} of decoded
// values for template processing. It round-trips through the types' JSON tags so the
// emitted keys match the CRD field names exactly - the same representation the controller
// decodes the embedded trait spec back into. Returns nil on error.
func apiSliceToInterface(v interface{}) []interface{} {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// buildValidationFields returns the CEL validation rules keyed by their CRD json tags
// (validations, preRenderValidations, postRenderValidations) for template processing, or
// nil when none are set. Trait/ClusterTrait and ComponentType/ClusterComponentType share
// this exact shape, so they all reconstruct it here rather than field by field, keeping the
// four call sites from drifting apart. Callers pass the deprecated Validations slice
// explicitly (with a //nolint at the read site); it is carried through only for backward
// compatibility.
func buildValidationFields(validations, preRenderValidations []v1alpha1.ValidationRule,
	postRenderValidations []v1alpha1.PostRenderValidation) map[string]interface{} {
	fields := make(map[string]interface{})
	if len(validations) > 0 {
		fields["validations"] = apiSliceToInterface(validations)
	}
	if len(preRenderValidations) > 0 {
		fields["preRenderValidations"] = apiSliceToInterface(preRenderValidations)
	}
	if len(postRenderValidations) > 0 {
		fields["postRenderValidations"] = apiSliceToInterface(postRenderValidations)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// TraitRef is a convenience type for trait references
// This mirrors the v1alpha1.ComponentTrait structure but uses map for Parameters
type TraitRef struct {
	Kind         string
	Name         string
	InstanceName string
	Parameters   map[string]interface{}
}
