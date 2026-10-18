// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package endpoint holds validation shared by the Endpoint-family controllers
// and webhooks.
package endpoint

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/schema"
)

// ValidateValues checks raw values (Endpoint.spec.parameters or
// EndpointReleaseBinding.spec.endpointTypeEnvironmentConfigs) against a schema
// section declared on an (Cluster)EndpointType. Schema defaults are applied
// before validation, as the render pipeline does, so a required field with a
// default is satisfied without the author repeating it.
//
// A nil or empty section accepts any values: the type declares no schema, and
// the values are then unused by its templates.
func ValidateValues(section *v1alpha1.SchemaSection, raw *runtime.RawExtension) error {
	rawSchema := section.GetRaw()
	if rawSchema == nil || len(rawSchema.Raw) == 0 {
		return nil
	}

	structural, jsonSchema, err := schema.ResolveSectionToBundle(section)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}
	if jsonSchema == nil {
		return nil
	}

	values := map[string]any{}
	if raw != nil && len(raw.Raw) > 0 {
		if err := json.Unmarshal(raw.Raw, &values); err != nil {
			return fmt.Errorf("failed to parse values: %w", err)
		}
	}

	values = schema.ApplyDefaults(values, structural)
	return schema.ValidateWithJSONSchema(values, jsonSchema)
}
