// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	apiservercel "k8s.io/apiserver/pkg/cel"
)

// forEachType represents the type of iteration in a forEach expression
type forEachType int

const (
	// forEachUnknown indicates the forEach expression type cannot be determined
	forEachUnknown forEachType = iota
	// forEachList indicates forEach iterates over a list
	forEachList
	// forEachMap indicates forEach iterates over a map
	forEachMap
)

// String returns the string representation of forEachType
func (t forEachType) String() string {
	switch t {
	case forEachList:
		return "list"
	case forEachMap:
		return "map"
	default:
		return "unknown"
	}
}

// forEachInfo contains information about a forEach expression and its loop variable
type forEachInfo struct {
	// Type indicates whether forEach iterates over a list or map
	Type forEachType

	// VarName is the name of the loop variable (default: "item")
	VarName string

	// VarType is the CEL type of the loop variable
	// For maps: ObjectType with "key" and "value" fields
	// For lists: The element type
	VarType *cel.Type

	// VarDeclType is the DeclType for the loop variable (only for forEachMap).
	// Used to register a proper object type with key/value fields so that
	// CEL can validate field access on the loop variable.
	VarDeclType *apiservercel.DeclType

	// KeyType is the type of map keys (only for forEachMap)
	KeyType *cel.Type

	// ValueType is the type of map values (only for forEachMap)
	ValueType *cel.Type

	// ElementType is the type of list elements (only for forEachList)
	ElementType *cel.Type
}

// analyzeForEachExpression analyzes a forEach expression to determine iteration type and variable types.
// This allows proper type checking of the loop variable usage within the forEach body.
//
// For map iteration, the loop variable will have .key and .value fields.
// For list iteration, the loop variable type matches the list element type.
func analyzeForEachExpression(forEachExpr string, varName string, env *cel.Env) (*forEachInfo, error) {
	if varName == "" {
		varName = "item" // Default loop variable name
	}

	// Parse the forEach expression
	ast, issues := env.Parse(forEachExpr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("invalid forEach expression: %w", issues.Err())
	}

	// Type-check to determine output type
	checked, issues := env.Check(ast)
	if issues != nil && issues.Err() != nil {
		// The expression might reference dynamic variables, which is OK
		// We'll return unknown type in that case
		return &forEachInfo{
			Type:    forEachUnknown,
			VarName: varName,
			VarType: cel.DynType,
		}, nil
	}

	outputType := checked.OutputType()
	info := &forEachInfo{VarName: varName}

	// Determine iteration type based on output type
	switch outputType.Kind() {
	case types.MapKind:
		// Map iteration: loop variable has .key and .value fields
		info.Type = forEachMap

		// Extract key and value types from map type parameters
		params := outputType.Parameters()
		if len(params) >= 2 {
			info.KeyType = params[0]
			info.ValueType = params[1]
		} else {
			// Fallback for dynamic maps
			info.KeyType = cel.StringType
			info.ValueType = cel.DynType
		}

		// MapEntry DeclType is created in extendEnvWithForEach where we have
		// access to the DeclTypeProvider for resolving the value's actual type.
		// For now, just record the value type for later resolution.
		info.VarType = cel.DynType // placeholder; replaced in extendEnvWithForEach

	case types.ListKind:
		// List iteration: loop variable type is the element type
		info.Type = forEachList

		params := outputType.Parameters()
		if len(params) > 0 {
			info.ElementType = params[0]
			info.VarType = params[0]
		} else {
			// Dynamic list - element type unknown
			info.ElementType = cel.DynType
			info.VarType = cel.DynType
		}

	default:
		// Unknown or dynamic type - be permissive
		info.Type = forEachUnknown
		info.VarType = cel.DynType
	}

	return info, nil
}

// extendEnvWithForEach extends a CEL environment with the forEach loop variable.
// For map iterations, creates a MapEntry DeclType with properly typed key/value fields
// so CEL validates field access on the loop variable. The typeProvider is used to
// resolve the map's value type for deep field validation.
func extendEnvWithForEach(env *cel.Env, info *forEachInfo, typeProvider *apiservercel.DeclTypeProvider) (*cel.Env, error) {
	if info == nil {
		return env, nil
	}

	if info.Type == forEachMap {
		return extendEnvWithMapForEach(env, info, typeProvider)
	}

	return env.Extend(
		cel.Variable(info.VarName, info.VarType),
	)
}

// extendEnvWithMapForEach creates a MapEntry DeclType with key (string) and value
// (resolved from the provider if possible, otherwise dyn) fields, registers it,
// and declares the loop variable with the proper type.
func extendEnvWithMapForEach(env *cel.Env, info *forEachInfo, typeProvider *apiservercel.DeclTypeProvider) (*cel.Env, error) {
	// Resolve the value's DeclType from the provider if the value type is a known object type
	valueDeclType := resolveValueDeclType(info.ValueType, typeProvider)

	mapEntryType := apiservercel.NewObjectType("MapEntry", map[string]*apiservercel.DeclField{
		"key":   apiservercel.NewDeclField("key", apiservercel.StringType, true, nil, nil),
		"value": apiservercel.NewDeclField("value", valueDeclType, true, nil, nil),
	})

	info.VarDeclType = mapEntryType
	info.VarType = mapEntryType.CelType()

	provider := apiservercel.NewDeclTypeProvider(mapEntryType)
	providerOpts, err := provider.EnvOptions(env.CELTypeProvider())
	if err != nil {
		return nil, fmt.Errorf("failed to create type provider for forEach variable: %w", err)
	}
	opts := make([]cel.EnvOption, 0, 1+len(providerOpts))
	opts = append(opts, cel.Variable(info.VarName, info.VarType))
	opts = append(opts, providerOpts...)
	return env.Extend(opts...)
}

// resolveValueDeclType creates a shadow DeclType for the map value that mirrors
// the original type's field structure but uses unique names to avoid conflicts
// with types already registered in the base environment. Recursively shadows
// nested object types so that field validation works at all depths.
func resolveValueDeclType(valueType *cel.Type, typeProvider *apiservercel.DeclTypeProvider) *apiservercel.DeclType {
	if valueType == nil || typeProvider == nil {
		return apiservercel.DynType
	}

	if valueType.Kind() != types.StructKind {
		return apiservercel.DynType
	}

	typeName := valueType.String()
	srcType, found := typeProvider.FindDeclType(typeName)
	if !found || len(srcType.Fields) == 0 {
		return apiservercel.DynType
	}

	return shadowDeclType(srcType, "MapEntry.value", make(map[string]bool))
}

// shadowDeclType recursively creates a shadow copy of a DeclType with a unique
// name prefix. Nested object-type fields are shadowed recursively; primitive
// and other types fall back to DynType.
func shadowDeclType(src *apiservercel.DeclType, shadowName string, seen map[string]bool) *apiservercel.DeclType {
	if seen[src.TypeName()] {
		return apiservercel.DynType
	}
	seen[src.TypeName()] = true

	fields := make(map[string]*apiservercel.DeclField, len(src.Fields))
	for name, field := range src.Fields {
		fieldType := apiservercel.DynType
		if field.Type != nil && field.Type.IsObject() && len(field.Type.Fields) > 0 {
			fieldType = shadowDeclType(field.Type, shadowName+"."+name, seen)
		}
		fields[name] = apiservercel.NewDeclField(name, fieldType, field.Required, nil, nil)
	}
	return apiservercel.NewObjectType(shadowName, fields)
}
