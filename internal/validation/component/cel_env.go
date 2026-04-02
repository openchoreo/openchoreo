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
// Derived from context.FunctionReturnTypes() so the type list stays in sync
// with CELExtensions() and CELValidationExtensions() automatically.
var functionReturnDeclTypes = func() []*apiservercel.DeclType {
	returnTypes := context.FunctionReturnTypes()
	result := make([]*apiservercel.DeclType, len(returnTypes))
	for i, t := range returnTypes {
		result[i] = decltype.FromGoType(t)
	}
	return result
}()

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

// buildComponentCELEnv creates a schema-aware CEL environment for component validation.
func buildComponentCELEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	return buildCELEnv(componentContextFields, opts)
}

// buildTraitCELEnv creates a schema-aware CEL environment for trait validation.
func buildTraitCELEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	return buildCELEnv(traitContextFields, opts)
}

// buildCELEnv creates a schema-aware CEL environment with the given context fields and schema options.
// Schema-based fields (parameters, environmentConfigs) are derived from the provided schemas.
// Reflection-based fields (metadata, workload, etc.) come from the contextFields slice.
func buildCELEnv(contextFields []decltype.FieldInfo, opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	baseEnv, err := createBaseEnv()
	if err != nil {
		return nil, nil, err
	}

	numFields := len(contextFields) + len(schemaBasedFields)
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
	for _, f := range contextFields {
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
func createBaseEnv() (*cel.Env, error) {
	baseEnvOpts := template.BaseCELExtensions()
	baseEnvOpts = append(baseEnvOpts, context.CELValidationExtensions()...)
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
