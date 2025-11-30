// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// ResourceType indicates which type of resource is being validated
type ResourceType int

const (
	// ComponentTypeResource indicates validation for ComponentType resources
	ComponentTypeResource ResourceType = iota
	// TraitResource indicates validation for Trait resources
	TraitResource
)

// CELValidator provides comprehensive CEL expression validation with AST analysis
type CELValidator struct {
	baseEnv      *cel.Env
	resourceType ResourceType
	allowedRoots map[string]bool
}

// NewCELValidator creates a validator for the specified resource type.
// The validator uses different environments for ComponentType vs Trait resources
// to enforce proper variable access (e.g., traits can't access workload).
func NewCELValidator(resourceType ResourceType) (*CELValidator, error) {
	var env *cel.Env
	var allowedRoots []string
	var err error

	switch resourceType {
	case ComponentTypeResource:
		env, err = BuildComponentCELEnv()
		allowedRoots = ComponentAllowedVariables
	case TraitResource:
		env, err = BuildTraitCELEnv()
		allowedRoots = TraitAllowedVariables
	default:
		return nil, fmt.Errorf("unknown resource type: %v", resourceType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Build allowed roots map for quick lookup
	rootsMap := make(map[string]bool)
	for _, root := range allowedRoots {
		rootsMap[root] = true
	}

	return &CELValidator{
		baseEnv:      env,
		resourceType: resourceType,
		allowedRoots: rootsMap,
	}, nil
}

// ValidateExpression validates a CEL expression with the base environment
func (v *CELValidator) ValidateExpression(expr string) error {
	return v.ValidateWithEnv(expr, v.baseEnv)
}

// ValidateWithEnv validates a CEL expression with a specific environment.
// This is used when the environment has been extended with forEach variables.
func (v *CELValidator) ValidateWithEnv(expr string, env *cel.Env) error {
	// Parse the expression
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("parse error: %w", issues.Err())
	}

	// Type check the expression
	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("type check error: %w", issues.Err())
	}

	// Validate variable references using AST analysis
	// Note: We don't validate against allowedRoots when using extended env
	// because forEach variables are dynamically added
	if env == v.baseEnv {
		return v.validateVariableReferences(checked)
	}

	return nil
}

// ValidateBooleanExpression ensures the expression returns a boolean value.
// Used for includeWhen conditions and where filters.
func (v *CELValidator) ValidateBooleanExpression(expr string, env *cel.Env) error {
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("parse error: %w", issues.Err())
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("type check error: %w", issues.Err())
	}

	// Verify output type is boolean
	outputType := checked.OutputType()
	if !outputType.IsExactType(cel.BoolType) && outputType != cel.DynType {
		return fmt.Errorf("expression must return boolean, got %s", outputType)
	}

	// Validate variable references if using base environment
	if env == v.baseEnv {
		return v.validateVariableReferences(checked)
	}

	return nil
}

// ValidateIterableExpression ensures the expression returns a list or map (for forEach).
func (v *CELValidator) ValidateIterableExpression(expr string, env *cel.Env) error {
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("parse error: %w", issues.Err())
	}

	checked, issues := env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("type check error: %w", issues.Err())
	}

	// Verify output type is iterable (list or map)
	outputType := checked.OutputType()
	kind := outputType.Kind()

	// Allow DynType since it could be iterable at runtime
	// Reject known non-iterable types (strings, numbers, booleans, etc.)
	if kind != types.ListKind && kind != types.MapKind && outputType != cel.DynType {
		return fmt.Errorf("forEach expression must return list or map, got %s", outputType)
	}

	return nil
}

// validateVariableReferences performs basic validation of variable references
// The CEL Check phase already does most of the validation, so we keep this simple
func (v *CELValidator) validateVariableReferences(checkedAst *cel.Ast) error {
	// For now, we rely on the CEL type checker to validate variable references
	// The type checker will catch undefined variables and type mismatches
	// We could enhance this later if needed to provide more specific error messages
	return nil
}

// GetBaseEnv returns the base CEL environment for this validator
func (v *CELValidator) GetBaseEnv() *cel.Env {
	return v.baseEnv
}
