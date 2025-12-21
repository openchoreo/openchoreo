// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package decltype

import (
	"reflect"
	"strings"

	apiservercel "k8s.io/apiserver/pkg/cel"
)

// FieldInfo holds metadata about a struct field for CEL type registration.
type FieldInfo struct {
	Name     string
	DeclType *apiservercel.DeclType
}

// ExtractFields extracts field metadata from a struct type for CEL registration.
// Fields in skip are excluded from the result.
func ExtractFields(t reflect.Type, skip map[string]bool) []FieldInfo {
	var fields []FieldInfo
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonName := getJSONFieldName(field)
		if jsonName == "" || jsonName == "-" || skip[jsonName] {
			continue
		}

		fieldType := fromGoType(field.Type)
		if fieldType.IsObject() {
			fieldType = fieldType.MaybeAssignTypeName(capitalizeFirst(jsonName))
		}

		fields = append(fields, FieldInfo{Name: jsonName, DeclType: fieldType})
	}
	return fields
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
