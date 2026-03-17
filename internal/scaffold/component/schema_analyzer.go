// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// GetFieldTypeOrder returns the sort order for a field type.
// Lower numbers come first: primitives (0-2), objects (3), maps (4), arrays (5).
// This is used to sort fields in a consistent order across generators.
func GetFieldTypeOrder(prop extv1.JSONSchemaProps) int {
	switch prop.Type {
	case typeBoolean:
		return 0
	case typeInteger, typeNumber:
		return 1
	case typeString:
		return 2
	case typeObject:
		// Objects with defined properties come first; everything else is treated as maps
		if len(prop.Properties) > 0 {
			return 3 // object (has structure)
		}
		return 4 // map or empty object
	case typeArray:
		return 5
	default:
		return 6 // unknown types last
	}
}
