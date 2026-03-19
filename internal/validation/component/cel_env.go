// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"reflect"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/internal/validation/component/decltype"
)

// schemaBasedFields are populated from user-provided schemas, not reflection.
var schemaBasedFields = map[string]bool{
	"parameters":         true,
	"environmentConfigs": true,
}

// Cached field info derived from context types (excludes schema-based fields).
var (
	componentContextFields = decltype.ExtractFields(reflect.TypeFor[context.ComponentContext](), schemaBasedFields)
	traitContextFields     = decltype.ExtractFields(reflect.TypeFor[context.TraitContext](), schemaBasedFields)
)

// functionReturnDeclTypes are DeclTypes for the return types of CEL helper functions.
// Registered with the DeclTypeProvider so the type checker can resolve field access
// on forEach loop variables that iterate over function results.
var functionReturnDeclTypes = []*apiservercel.DeclType{
	decltype.FromGoType(reflect.TypeFor[context.ConfigFileListEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.SecretFileListEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.EnvFromEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.VolumeMountEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.VolumeEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.EnvsByContainerEntry]()),
	decltype.FromGoType(reflect.TypeFor[context.ServicePortEntry]()),
}

// SchemaOptions provides schema configuration for CEL environment and validation.
// Used by both component and trait CEL environments.
type SchemaOptions struct {
	// ParametersSchema is the structural schema for parameters.
	// If nil, an empty object type will be used.
	ParametersSchema *apiextschema.Structural

	// EnvironmentConfigsSchema is the structural schema for environmentConfigs.
	// If nil, an empty object type will be used.
	EnvironmentConfigsSchema *apiextschema.Structural
}

// BuildComponentCELEnv creates a schema-aware CEL environment for component validation.
// Variables are derived from ComponentContext struct fields:
//   - parameters, environmentConfigs: Schema-aware types (or empty object if not provided)
//   - metadata, dataplane, workload, configurations: Types derived via reflection
//
// Returns the environment and the DeclTypeProvider (for forEach value type resolution).
func BuildComponentCELEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	baseEnv, err := createBaseEnv(true)
	if err != nil {
		return nil, nil, err
	}

	numFields := len(componentContextFields) + len(schemaBasedFields)
	declTypes := make([]*apiservercel.DeclType, 0, numFields+len(functionReturnDeclTypes))
	varOpts := make([]cel.EnvOption, 0, numFields)

	// Register schema-based fields
	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")
	declTypes = append(declTypes, paramType)
	varOpts = append(varOpts, cel.Variable("parameters", paramType.CelType()))

	environmentConfigsType := schemaToTypeOrEmpty(opts.EnvironmentConfigsSchema, "EnvironmentConfigs")
	declTypes = append(declTypes, environmentConfigsType)
	varOpts = append(varOpts, cel.Variable("environmentConfigs", environmentConfigsType.CelType()))

	// Register reflection-based fields
	for _, f := range componentContextFields {
		declTypes = append(declTypes, f.DeclType)
		varOpts = append(varOpts, cel.Variable(f.Name, f.DeclType.CelType()))
	}

	// Register function return types so the type checker can validate field access
	declTypes = append(declTypes, functionReturnDeclTypes...)

	provider := apiservercel.NewDeclTypeProvider(declTypes...)
	providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
	if err != nil {
		return nil, nil, err
	}
	varOpts = append(varOpts, providerOpts...)

	env, err := baseEnv.Extend(varOpts...)
	if err != nil {
		return nil, nil, err
	}
	return env, provider, nil
}

// BuildTraitCELEnv creates a schema-aware CEL environment for trait validation.
// Variables are derived from TraitContext struct fields:
//   - parameters, environmentConfigs: Schema-aware types (or empty object if not provided)
//   - trait, metadata, dataplane, workload, configurations: Types derived via reflection
//
// Returns the environment and the DeclTypeProvider (for forEach value type resolution).
func BuildTraitCELEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	baseEnv, err := createBaseEnv(true)
	if err != nil {
		return nil, nil, err
	}

	numFields := len(traitContextFields) + len(schemaBasedFields)
	declTypes := make([]*apiservercel.DeclType, 0, numFields+len(functionReturnDeclTypes))
	varOpts := make([]cel.EnvOption, 0, numFields)

	// Register schema-based fields
	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")
	declTypes = append(declTypes, paramType)
	varOpts = append(varOpts, cel.Variable("parameters", paramType.CelType()))

	environmentConfigsType := schemaToTypeOrEmpty(opts.EnvironmentConfigsSchema, "EnvironmentConfigs")
	declTypes = append(declTypes, environmentConfigsType)
	varOpts = append(varOpts, cel.Variable("environmentConfigs", environmentConfigsType.CelType()))

	// Register reflection-based fields
	for _, f := range traitContextFields {
		declTypes = append(declTypes, f.DeclType)
		varOpts = append(varOpts, cel.Variable(f.Name, f.DeclType.CelType()))
	}

	// Register function return types so the type checker can validate field access
	declTypes = append(declTypes, functionReturnDeclTypes...)

	provider := apiservercel.NewDeclTypeProvider(declTypes...)
	providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
	if err != nil {
		return nil, nil, err
	}
	varOpts = append(varOpts, providerOpts...)

	env, err := baseEnv.Extend(varOpts...)
	if err != nil {
		return nil, nil, err
	}
	return env, provider, nil
}

// createBaseEnv creates the base CEL environment with standard extensions.
// Uses CELValidationExtensions() which provides typed function return types
// so the type checker can validate field access on forEach loop variables.
func createBaseEnv(includeConfigExtensions bool) (*cel.Env, error) {
	baseEnvOpts := template.BaseCELExtensions()

	if includeConfigExtensions {
		baseEnvOpts = append(baseEnvOpts, context.CELValidationExtensions()...)
	}

	return cel.NewEnv(baseEnvOpts...)
}

// schemaToTypeOrEmpty converts a structural schema to a DeclType,
// returning an empty object type if schema is nil or conversion fails.
func schemaToTypeOrEmpty(schema *apiextschema.Structural, typeName string) *apiservercel.DeclType {
	if schema != nil {
		if dt := model.SchemaDeclType(schema, false); dt != nil {
			return dt.MaybeAssignTypeName(typeName)
		}
	}
	return apiservercel.NewObjectType(typeName, map[string]*apiservercel.DeclField{})
}
