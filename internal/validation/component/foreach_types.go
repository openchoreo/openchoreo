// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// ForEachType represents the type of iteration in a forEach expression
type ForEachType int

const (
	// ForEachUnknown indicates the forEach expression type cannot be determined
	ForEachUnknown ForEachType = iota
	// ForEachList indicates forEach iterates over a list
	ForEachList
	// ForEachMap indicates forEach iterates over a map
	ForEachMap
)

// String returns the string representation of ForEachType
func (t ForEachType) String() string {
	switch t {
	case ForEachList:
		return "list"
	case ForEachMap:
		return "map"
	default:
		return "unknown"
	}
}

// ForEachInfo contains information about a forEach expression and its loop variable
type ForEachInfo struct {
	// Type indicates whether forEach iterates over a list or map
	Type ForEachType

	// VarName is the name of the loop variable (default: "item")
	VarName string

	// VarType is the CEL type of the loop variable
	// For maps: ObjectType with "key" and "value" fields
	// For lists: The element type
	VarType *cel.Type

	// KeyType is the type of map keys (only for ForEachMap)
	KeyType *cel.Type

	// ValueType is the type of map values (only for ForEachMap)
	ValueType *cel.Type

	// ElementType is the type of list elements (only for ForEachList)
	ElementType *cel.Type
}

// AnalyzeForEachExpression analyzes a forEach expression to determine iteration type and variable types.
// This allows proper type checking of the loop variable usage within the forEach body.
//
// For map iteration, the loop variable will have .key and .value fields.
// For list iteration, the loop variable type matches the list element type.
func AnalyzeForEachExpression(forEachExpr string, varName string, env *cel.Env) (*ForEachInfo, error) {
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
		return &ForEachInfo{
			Type:    ForEachUnknown,
			VarName: varName,
			VarType: cel.DynType,
		}, nil
	}

	outputType := checked.OutputType()
	info := &ForEachInfo{VarName: varName}

	// Determine iteration type based on output type
	switch outputType.Kind() {
	case types.MapKind:
		// Map iteration: loop variable has .key and .value fields
		info.Type = ForEachMap

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

		// Create MapEntry type for the loop variable
		// This matches CEL's behavior when iterating over maps
		// For now, we'll use DynType since cel.ObjectType has different signature
		// In practice, the map iteration will still work correctly
		info.VarType = cel.DynType

	case types.ListKind:
		// List iteration: loop variable type is the element type
		info.Type = ForEachList

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
		info.Type = ForEachUnknown
		info.VarType = cel.DynType
	}

	return info, nil
}

// ExtendEnvWithForEach extends a CEL environment with the forEach loop variable.
// The variable is typed appropriately based on the forEach analysis.
func ExtendEnvWithForEach(env *cel.Env, info *ForEachInfo) (*cel.Env, error) {
	if info == nil {
		return env, nil
	}

	return env.Extend(
		cel.Variable(info.VarName, info.VarType),
	)
}
