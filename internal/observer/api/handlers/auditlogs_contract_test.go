// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/observer/api/gen"
)

// specFilterProperty resolves a filter's path in the query vocabulary to the
// schema property it names, mirroring how the filters nest in the body.
func specFilterProperty(t *testing.T, schemas openapi3.Schemas, path string) *openapi3.Schema {
	t.Helper()

	schema, name := "AuditLogsQueryRequest", path
	switch {
	case strings.HasPrefix(path, "actor."):
		schema, name = "AuditLogsActorFilter", strings.TrimPrefix(path, "actor.")
	case strings.HasPrefix(path, "resource."):
		schema, name = "AuditLogsResourceFilter", strings.TrimPrefix(path, "resource.")
	}

	prop := schemas[schema].Value.Properties[name]
	require.NotNil(t, prop, "spec's %s has no %q property", schema, name)
	return prop.Value
}

// TestAuditLogsValidatorsMatchSpec ties the hand-written audit validators to
// openapi/observer-api.yaml.
//
// The observer mounts no request-validating middleware, so validations.go
// restates the spec's enums and bounds — and a restatement drifts. These read
// them back out of the spec so the drift fails here instead.
func TestAuditLogsValidatorsMatchSpec(t *testing.T) {
	t.Parallel()

	swagger, err := gen.GetSwagger()
	require.NoError(t, err, "failed to load the OpenAPI spec")
	schemas := swagger.Components.Schemas

	t.Run("filter item counts", func(t *testing.T) {
		for path, maxItems := range auditLogsFilterMaxItems {
			prop := specFilterProperty(t, schemas, path)
			require.NotNil(t, prop.MaxItems, "spec declares no maxItems for %s", path)
			assert.EqualValues(t, maxItems, *prop.MaxItems, "maxItems for %s", path)
			assert.Nil(t, prop.Items.Value.MaxLength,
				"%s declares a maxLength the validator no longer enforces", path)
		}
	})

	// An unlisted filter is rejected as unknown, so a filter added to the spec
	// and the request type but not the table would 400 rather than apply.
	t.Run("every spec filter is bounded", func(t *testing.T) {
		for schema, prefix := range map[string]string{
			"AuditLogsQueryRequest":   "",
			"AuditLogsActorFilter":    "actor.",
			"AuditLogsResourceFilter": "resource.",
		} {
			for name, prop := range schemas[schema].Value.Properties {
				if prop.Value.Type == nil || !prop.Value.Type.Is("array") {
					continue
				}
				assert.Contains(t, auditLogsFilterMaxItems, prefix+name)
			}
		}
	})

	t.Run("closed enums", func(t *testing.T) {
		for path, valid := range map[string]map[string]bool{
			"category": auditLogCategories,
			"result":   auditLogResults,
			"surface":  auditLogSurfaces,
		} {
			assert.Equal(t, specEnum(t, specFilterProperty(t, schemas, path).Items.Value), valid, path)
		}
	})

	t.Run("filter values vocabulary", func(t *testing.T) {
		filter := schemas["AuditLogFilterValuesRequest"].Value.Properties["filter"].Value
		assert.Equal(t, specEnum(t, filter), auditLogsFilterPaths)
	})
}

func specEnum(t *testing.T, schema *openapi3.Schema) map[string]bool {
	t.Helper()
	require.NotEmpty(t, schema.Enum, "spec declares no enum")

	out := map[string]bool{}
	for _, v := range schema.Enum {
		value, ok := v.(string)
		require.True(t, ok, "enum member %v is not a string", v)
		out[value] = true
	}
	return out
}
