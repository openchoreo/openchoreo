// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
)

// Variable names available in component rendering context
const (
	VarParameters     = "parameters"
	VarEnvOverrides   = "envOverrides"
	VarWorkload       = "workload"
	VarConfigurations = "configurations"
	VarComponent      = "component"
	VarMetadata       = "metadata"
	VarDataplane      = "dataplane"
)

// Variable names specific to trait rendering context
const (
	VarTrait = "trait"
	// VarParameters, VarEnvOverrides, VarComponent, VarMetadata are shared with component context
)

// ComponentAllowedVariables lists all variables available in component rendering
var ComponentAllowedVariables = []string{
	VarParameters,
	VarEnvOverrides,
	VarWorkload,
	VarConfigurations,
	VarComponent,
	VarMetadata,
	VarDataplane,
}

// TraitAllowedVariables lists all variables available in trait rendering
var TraitAllowedVariables = []string{
	VarParameters,
	VarEnvOverrides,
	VarTrait,
	VarComponent,
	VarMetadata,
	VarDataplane,
}

// ComponentCELEnvOptions configures the CEL environment for component validation.
type ComponentCELEnvOptions struct {
	// ParametersSchema is the structural schema for parameters (from ComponentType.Schema.Parameters).
	// If nil, DynType will be used.
	ParametersSchema *apiextschema.Structural

	// EnvOverridesSchema is the structural schema for envOverrides (from ComponentType.Schema.EnvOverrides).
	// If nil, DynType will be used.
	EnvOverridesSchema *apiextschema.Structural
}

// BuildComponentCELEnv creates a schema-aware CEL environment for component validation.
// This provides better type checking by using actual types for fixed-structure
// variables and user-defined schemas for parameters/envOverrides.
//
// The environment includes:
//   - parameters: Schema-aware type from ComponentType.Schema.Parameters (or empty object if not provided)
//   - envOverrides: Schema-aware type from ComponentType.Schema.EnvOverrides (or empty object if not provided)
//   - metadata: Typed from context.MetadataContext
//   - dataplane: Typed from context.DataPlaneData
//   - workload: Typed from context.WorkloadData
//   - configurations: Typed from context.ContainerConfigurationsMap
//   - component: Object type with name field
func BuildComponentCELEnv(opts ComponentCELEnvOptions) (*cel.Env, error) {
	// Create base environment with standard extensions
	baseEnvOpts := []cel.EnvOption{
		cel.OptionalTypes(),

		// Standard CEL extensions
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(),
		ext.Sets(),
		ext.TwoVarComprehensions(),
	}

	// Add OpenChoreo custom functions
	baseEnvOpts = append(baseEnvOpts, template.CustomFunctions()...)

	// Add configurations helper extensions (macros and functions)
	baseEnvOpts = append(baseEnvOpts, context.CELExtensions()...)

	baseEnv, err := cel.NewEnv(baseEnvOpts...)
	if err != nil {
		return nil, err
	}

	// Build variable declarations and collect DeclTypes for schema-aware variables
	var declTypes []*apiservercel.DeclType
	var varOpts []cel.EnvOption

	// Parameters: use schema if provided, otherwise empty object (no fields allowed)
	if opts.ParametersSchema != nil {
		paramType := model.SchemaDeclType(opts.ParametersSchema, false)
		if paramType != nil {
			// Assign a type name so CEL can resolve fields properly
			paramType = paramType.MaybeAssignTypeName("Parameters")
			declTypes = append(declTypes, paramType)
			varOpts = append(varOpts, cel.Variable(VarParameters, paramType.CelType()))
		} else {
			// Schema conversion failed, use empty object
			emptyParams := buildEmptyObjectType("Parameters")
			declTypes = append(declTypes, emptyParams)
			varOpts = append(varOpts, cel.Variable(VarParameters, emptyParams.CelType()))
		}
	} else {
		// No schema provided, use empty object (any parameters.* access will fail)
		emptyParams := buildEmptyObjectType("Parameters")
		declTypes = append(declTypes, emptyParams)
		varOpts = append(varOpts, cel.Variable(VarParameters, emptyParams.CelType()))
	}

	// EnvOverrides: use schema if provided, otherwise empty object (no fields allowed)
	if opts.EnvOverridesSchema != nil {
		envOverridesType := model.SchemaDeclType(opts.EnvOverridesSchema, false)
		if envOverridesType != nil {
			// Assign a type name so CEL can resolve fields properly
			envOverridesType = envOverridesType.MaybeAssignTypeName("EnvOverrides")
			declTypes = append(declTypes, envOverridesType)
			varOpts = append(varOpts, cel.Variable(VarEnvOverrides, envOverridesType.CelType()))
		} else {
			// Schema conversion failed, use empty object
			emptyEnvOverrides := buildEmptyObjectType("EnvOverrides")
			declTypes = append(declTypes, emptyEnvOverrides)
			varOpts = append(varOpts, cel.Variable(VarEnvOverrides, emptyEnvOverrides.CelType()))
		}
	} else {
		// No schema provided, use empty object (any envOverrides.* access will fail)
		emptyEnvOverrides := buildEmptyObjectType("EnvOverrides")
		declTypes = append(declTypes, emptyEnvOverrides)
		varOpts = append(varOpts, cel.Variable(VarEnvOverrides, emptyEnvOverrides.CelType()))
	}

	// Other variables use DynType for now (could be enhanced with reflection-based types later)
	varOpts = append(varOpts,
		cel.Variable(VarWorkload, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarConfigurations, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarComponent, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarMetadata, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarDataplane, cel.MapType(cel.StringType, cel.DynType)),
	)

	// If we have schema-aware types, create a DeclTypeProvider and get its env options
	if len(declTypes) > 0 {
		provider := apiservercel.NewDeclTypeProvider(declTypes...)
		providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
		if err != nil {
			return nil, err
		}
		varOpts = append(varOpts, providerOpts...)
	}

	// Extend base environment with variable declarations
	return baseEnv.Extend(varOpts...)
}

// TraitCELEnvOptions configures the CEL environment for trait validation.
type TraitCELEnvOptions struct {
	// ParametersSchema is the structural schema for parameters (from Trait.Schema.Parameters).
	// If nil, DynType will be used.
	ParametersSchema *apiextschema.Structural

	// EnvOverridesSchema is the structural schema for envOverrides (from Trait.Schema.EnvOverrides).
	// If nil, DynType will be used.
	EnvOverridesSchema *apiextschema.Structural
}

// BuildTraitCELEnv creates a schema-aware CEL environment for trait validation.
// This provides better type checking by using actual types for fixed-structure
// variables and user-defined schemas for parameters/envOverrides.
//
// The environment includes:
//   - parameters: Schema-aware type from Trait.Schema.Parameters (or empty object if not provided)
//   - envOverrides: Schema-aware type from Trait.Schema.EnvOverrides (or empty object if not provided)
//   - trait: Typed from context.TraitMetadata
//   - component: Object type with name field
//   - metadata: Typed from context.MetadataContext
//   - dataplane: Typed from context.DataPlaneData
//
// Note: Traits don't have access to workload or configurations
func BuildTraitCELEnv(opts TraitCELEnvOptions) (*cel.Env, error) {
	// Create base environment with standard extensions
	baseEnvOpts := []cel.EnvOption{
		cel.OptionalTypes(),

		// Standard CEL extensions
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(),
		ext.Sets(),
		ext.TwoVarComprehensions(),
	}

	// Add OpenChoreo custom functions
	baseEnvOpts = append(baseEnvOpts, template.CustomFunctions()...)

	baseEnv, err := cel.NewEnv(baseEnvOpts...)
	if err != nil {
		return nil, err
	}

	// Build variable declarations and collect DeclTypes for schema-aware variables
	var declTypes []*apiservercel.DeclType
	var varOpts []cel.EnvOption

	// Parameters: use schema if provided, otherwise empty object (no fields allowed)
	if opts.ParametersSchema != nil {
		paramType := model.SchemaDeclType(opts.ParametersSchema, false)
		if paramType != nil {
			// Assign a type name so CEL can resolve fields properly
			paramType = paramType.MaybeAssignTypeName("Parameters")
			declTypes = append(declTypes, paramType)
			varOpts = append(varOpts, cel.Variable(VarParameters, paramType.CelType()))
		} else {
			// Schema conversion failed, use empty object
			emptyParams := buildEmptyObjectType("Parameters")
			declTypes = append(declTypes, emptyParams)
			varOpts = append(varOpts, cel.Variable(VarParameters, emptyParams.CelType()))
		}
	} else {
		// No schema provided, use empty object (any parameters.* access will fail)
		emptyParams := buildEmptyObjectType("Parameters")
		declTypes = append(declTypes, emptyParams)
		varOpts = append(varOpts, cel.Variable(VarParameters, emptyParams.CelType()))
	}

	// EnvOverrides: use schema if provided, otherwise empty object (no fields allowed)
	if opts.EnvOverridesSchema != nil {
		envOverridesType := model.SchemaDeclType(opts.EnvOverridesSchema, false)
		if envOverridesType != nil {
			// Assign a type name so CEL can resolve fields properly
			envOverridesType = envOverridesType.MaybeAssignTypeName("EnvOverrides")
			declTypes = append(declTypes, envOverridesType)
			varOpts = append(varOpts, cel.Variable(VarEnvOverrides, envOverridesType.CelType()))
		} else {
			// Schema conversion failed, use empty object
			emptyEnvOverrides := buildEmptyObjectType("EnvOverrides")
			declTypes = append(declTypes, emptyEnvOverrides)
			varOpts = append(varOpts, cel.Variable(VarEnvOverrides, emptyEnvOverrides.CelType()))
		}
	} else {
		// No schema provided, use empty object (any envOverrides.* access will fail)
		emptyEnvOverrides := buildEmptyObjectType("EnvOverrides")
		declTypes = append(declTypes, emptyEnvOverrides)
		varOpts = append(varOpts, cel.Variable(VarEnvOverrides, emptyEnvOverrides.CelType()))
	}

	// Other variables use DynType for now (could be enhanced with reflection-based types later)
	varOpts = append(varOpts,
		cel.Variable(VarTrait, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarComponent, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarMetadata, cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable(VarDataplane, cel.MapType(cel.StringType, cel.DynType)),
	)

	// If we have schema-aware types, create a DeclTypeProvider and get its env options
	if len(declTypes) > 0 {
		provider := apiservercel.NewDeclTypeProvider(declTypes...)
		providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
		if err != nil {
			return nil, err
		}
		varOpts = append(varOpts, providerOpts...)
	}

	// Extend base environment with variable declarations
	return baseEnv.Extend(varOpts...)
}

// buildEmptyObjectType builds an empty object DeclType with no fields.
// Any field access on this type will fail validation.
func buildEmptyObjectType(name string) *apiservercel.DeclType {
	return apiservercel.NewObjectType(name, map[string]*apiservercel.DeclField{})
}
