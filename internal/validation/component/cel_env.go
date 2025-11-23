// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"

	"github.com/openchoreo/openchoreo/internal/template"
)

// Variable names available in component rendering context
const (
	VarParameters     = "parameters"
	VarWorkload       = "workload"
	VarConfigurations = "configurations"
	VarComponent      = "component"
	VarMetadata       = "metadata"
	VarDataplane      = "dataplane"
)

// Variable names specific to trait rendering context
const (
	VarTrait = "trait"
	// VarParameters, VarComponent, VarMetadata are shared with component context
)

// ComponentAllowedVariables lists all variables available in component rendering
var ComponentAllowedVariables = []string{
	VarParameters,
	VarWorkload,
	VarConfigurations,
	VarComponent,
	VarMetadata,
	VarDataplane,
}

// TraitAllowedVariables lists all variables available in trait rendering
var TraitAllowedVariables = []string{
	VarParameters,
	VarTrait,
	VarComponent,
	VarMetadata,
}

// BuildComponentCELEnv creates a CEL environment for validating component resources.
// This environment matches the context created by BuildComponentContext in the pipeline.
func BuildComponentCELEnv() (*cel.Env, error) {
	envOpts := []cel.EnvOption{
		cel.OptionalTypes(),

		// Component context variables - matching internal/pipeline/component/context/component.go
		cel.Variable(VarParameters, cel.DynType),                                  // Component parameters merged with env overrides
		cel.Variable(VarWorkload, cel.MapType(cel.StringType, cel.DynType)),       // Workload spec
		cel.Variable(VarConfigurations, cel.MapType(cel.StringType, cel.DynType)), // Config/secret refs
		cel.Variable(VarComponent, cel.MapType(cel.StringType, cel.DynType)),      // Component metadata
		cel.Variable(VarMetadata, cel.MapType(cel.StringType, cel.DynType)),       // Resource generation metadata
		cel.Variable(VarDataplane, cel.MapType(cel.StringType, cel.DynType)),      // DataPlane context

		// Standard CEL extensions
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(),
		ext.Sets(),
		ext.TwoVarComprehensions(),
	}

	// Add OpenChoreo custom functions
	envOpts = append(envOpts, template.CustomFunctions()...)

	return cel.NewEnv(envOpts...)
}

// BuildTraitCELEnv creates a CEL environment for validating trait resources.
// This environment matches the context created by BuildTraitContext in the pipeline.
func BuildTraitCELEnv() (*cel.Env, error) {
	envOpts := []cel.EnvOption{
		cel.OptionalTypes(),

		// Trait context variables - matching internal/pipeline/component/context/trait.go
		// Note: Traits don't have access to workload, configurations, or dataplane
		cel.Variable(VarParameters, cel.DynType),                             // Trait parameters merged with env overrides
		cel.Variable(VarTrait, cel.MapType(cel.StringType, cel.DynType)),     // Trait metadata
		cel.Variable(VarComponent, cel.MapType(cel.StringType, cel.DynType)), // Component reference
		cel.Variable(VarMetadata, cel.MapType(cel.StringType, cel.DynType)),  // Resource generation metadata

		// Standard CEL extensions
		ext.Strings(),
		ext.Encoders(),
		ext.Math(),
		ext.Lists(),
		ext.Sets(),
		ext.TwoVarComprehensions(),
	}

	// Add OpenChoreo custom functions
	envOpts = append(envOpts, template.CustomFunctions()...)

	return cel.NewEnv(envOpts...)
}
