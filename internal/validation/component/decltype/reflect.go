// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package decltype

import (
	"reflect"
	"strings"

	apiservercel "k8s.io/apiserver/pkg/cel"
)

const (
	maxListSize = 1000
	maxMapSize  = 1000
)

// fromGoType converts a Go type to a CEL DeclType using reflection.
// This is used for fixed-structure types like MetadataContext, WorkloadData, etc.
func fromGoType(t reflect.Type) *apiservercel.DeclType {
	return typeToDecl(t, make(map[reflect.Type]*apiservercel.DeclType))
}

func typeToDecl(t reflect.Type, seen map[reflect.Type]*apiservercel.DeclType) *apiservercel.DeclType {
	// Handle pointers
	if t.Kind() == reflect.Ptr {
		return typeToDecl(t.Elem(), seen)
	}

	// Handle cycles
	if existing, ok := seen[t]; ok {
		return existing
	}

	switch t.Kind() {
	case reflect.String:
		return apiservercel.StringType
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return apiservercel.IntType
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return apiservercel.UintType
	case reflect.Float32, reflect.Float64:
		return apiservercel.DoubleType
	case reflect.Bool:
		return apiservercel.BoolType

	case reflect.Slice:
		elemType := typeToDecl(t.Elem(), seen)
		return apiservercel.NewListType(elemType, maxListSize)

	case reflect.Map:
		keyType := typeToDecl(t.Key(), seen)
		elemType := typeToDecl(t.Elem(), seen)
		return apiservercel.NewMapType(keyType, elemType, maxMapSize)

	case reflect.Struct:
		return structToDecl(t, seen)

	case reflect.Interface:
		// interface{} or any → DynType
		return apiservercel.DynType

	default:
		return apiservercel.DynType
	}
}

func structToDecl(t reflect.Type, seen map[reflect.Type]*apiservercel.DeclType) *apiservercel.DeclType {
	// Create placeholder to handle cycles
	placeholder := &apiservercel.DeclType{}
	seen[t] = placeholder

	fields := make(map[string]*apiservercel.DeclField)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag name
		jsonName := getJSONFieldName(field)
		if jsonName == "-" {
			continue
		}

		fieldType := typeToDecl(field.Type, seen)
		required := !isOmitempty(field)

		fields[jsonName] = apiservercel.NewDeclField(jsonName, fieldType, required, nil, nil)
	}

	objType := apiservercel.NewObjectType(t.Name(), fields)

	// Update placeholder
	*placeholder = *objType
	return placeholder
}

func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}
	return name
}

func isOmitempty(field reflect.StructField) bool {
	tag := field.Tag.Get("json")
	return strings.Contains(tag, "omitempty")
}
