// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
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
}

// CELValidatorSchemaOptions configures schema information for the validator.
type CELValidatorSchemaOptions struct {
	// ParametersSchema is the structural schema for parameters.
	ParametersSchema *apiextschema.Structural

	// EnvOverridesSchema is the structural schema for envOverrides.
	EnvOverridesSchema *apiextschema.Structural
}

// NewCELValidator creates a validator for the specified resource type.
// The validator uses different environments for ComponentType vs Trait resources
// to enforce proper variable access (e.g., traits can't access workload).
// Provides schema-aware type checking when schemas are provided in opts.
func NewCELValidator(resourceType ResourceType, opts CELValidatorSchemaOptions) (*CELValidator, error) {
	var env *cel.Env
	var err error

	switch resourceType {
	case ComponentTypeResource:
		env, err = BuildComponentCELEnv(ComponentCELEnvOptions(opts))
	case TraitResource:
		env, err = BuildTraitCELEnv(TraitCELEnvOptions(opts))
	default:
		return nil, fmt.Errorf("unknown resource type: %v", resourceType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &CELValidator{
		baseEnv:      env,
		resourceType: resourceType,
	}, nil
}

// ValidateExpression validates a CEL expression with the base environment
func (v *CELValidator) ValidateExpression(expr string) error {
	return v.ValidateWithEnv(expr, v.baseEnv)
}

// ValidateWithEnv validates a CEL expression with a specific environment.
// This is used when the environment has been extended with forEach variables.
func (v *CELValidator) ValidateWithEnv(expr string, env *cel.Env) error {
	parsed, issues := env.Parse(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("parse error: %w", issues.Err())
	}

	_, issues = env.Check(parsed)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("type check error: %w", issues.Err())
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

	outputType := checked.OutputType()
	if !outputType.IsExactType(cel.BoolType) && outputType != cel.DynType {
		return fmt.Errorf("expression must return boolean, got %s", outputType)
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

	outputType := checked.OutputType()
	kind := outputType.Kind()

	// Allow DynType since it could be iterable at runtime
	if kind != types.ListKind && kind != types.MapKind && outputType != cel.DynType {
		return fmt.Errorf("forEach expression must return list or map, got %s", outputType)
	}

	return nil
}

// GetBaseEnv returns the base CEL environment for this validator
func (v *CELValidator) GetBaseEnv() *cel.Env {
	return v.baseEnv
}
