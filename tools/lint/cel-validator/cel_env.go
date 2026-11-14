// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package celvalidator builds schema-aware CEL environments used to validate
// ${...} expressions in OpenChoreo templates.
//
// The env construction here is forked from
// internal/validation/component/cel_env.go so that tools/lint does not have to
// depend on the webhook validation layer. The two must be kept in sync when
// the CEL context surface (ComponentContext/TraitContext, the receiver macros,
// or the base extensions) changes.
package celvalidator

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	projectpipeline "github.com/openchoreo/openchoreo/internal/pipeline/project"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/internal/validation/component/decltype"
)

// schemaBasedFields are populated from user-provided schemas, not reflection.
var schemaBasedFields = map[string]bool{
	fieldParameters:         true,
	fieldEnvironmentConfigs: true,
}

// Cached field info derived from context types (excludes schema-based fields).
var (
	componentContextFields = decltype.ExtractFields(reflect.TypeFor[context.ComponentContext](), schemaBasedFields)
	traitContextFields     = decltype.ExtractFields(reflect.TypeFor[context.TraitContext](), schemaBasedFields)
	resourceContextFields  = decltype.ExtractFields(reflect.TypeFor[resourcepipeline.BaseContext](), schemaBasedFields)
)

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

// NewComponentEnv creates a schema-aware CEL environment over
// context.ComponentContext, the environment every ${...} expression in a
// ComponentType or ClusterComponentType resource template is checked against.
//
// The returned DeclTypeProvider must be handed to the caller because the
// reflected context types are only resolvable through it: registering
// cel.Variable alone would leave nested field access such as
// workload.container.image unresolved by the checker.
func NewComponentEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	return buildCELEnv(componentContextFields, opts)
}

// NewTraitEnv creates the equivalent environment over context.TraitContext.
// It is identical to NewComponentEnv except that trait expressions also see
// the "trait" variable (trait.name, trait.instanceName).
func NewTraitEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	return buildCELEnv(traitContextFields, opts)
}

// NewResourceEnv creates a schema-aware CEL environment over
// resourcepipeline.BaseContext for validating ResourceType and
// ClusterResourceType templates. Unlike NewComponentEnv/NewTraitEnv it returns
// no DeclTypeProvider: the resource path resolves every nested type through the
// provider options already merged into the env, so the caller has no use for a
// handle on it. Mirrors internal/validation/resource/cel_env.go.
//
// "applied" is intentionally not in scope here; call WithApplied to layer it on
// when validating outputs and readyWhen expressions.
func NewResourceEnv(opts SchemaOptions) (*cel.Env, error) {
	baseEnv, err := createResourceBaseEnv()
	if err != nil {
		return nil, err
	}

	numFields := len(resourceContextFields) + len(schemaBasedFields)
	declTypes := make([]*apiservercel.DeclType, 0, numFields)
	varOpts := make([]cel.EnvOption, 0, numFields)

	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")
	declTypes = append(declTypes, paramType)
	varOpts = append(varOpts, cel.Variable(fieldParameters, paramType.CelType()))

	envConfigsType := schemaToTypeOrEmpty(opts.EnvironmentConfigsSchema, "EnvironmentConfigs")
	declTypes = append(declTypes, envConfigsType)
	varOpts = append(varOpts, cel.Variable(fieldEnvironmentConfigs, envConfigsType.CelType()))

	for _, f := range resourceContextFields {
		declTypes = append(declTypes, f.DeclType)
		varOpts = append(varOpts, cel.Variable(f.Name, f.DeclType.CelType()))
	}

	provider := apiservercel.NewDeclTypeProvider(declTypes...)
	providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
	if err != nil {
		return nil, err
	}
	varOpts = append(varOpts, providerOpts...)

	return baseEnv.Extend(varOpts...)
}

// NewWorkflowEnv creates a schema-aware CEL environment for Workflow and
// ClusterWorkflow templates. The declared surface mirrors
// internal/pipeline/workflow.(*Pipeline).BuildCELContext exactly:
//
//   - parameters from spec.parameters.openAPIV3Schema
//   - metadata with namespaceName / workflowRunName / namespace (the enforced
//     execution namespace) / labels
//   - workflowplane.secretStore
//   - externalRefs, deliberately dyn-typed because its entries are external CR
//     specs that are only resolved at runtime
//
// There is no environmentConfigs variable and none of the component receiver
// macros: workflow templates are rendered against a plain map context, so only
// the base template extensions are in scope.
func NewWorkflowEnv(opts SchemaOptions) (*cel.Env, error) {
	baseEnv, err := cel.NewEnv(template.BaseCELExtensions()...)
	if err != nil {
		return nil, err
	}

	metadataType := apiservercel.NewObjectType("WorkflowMetadata", map[string]*apiservercel.DeclField{
		"namespaceName":   apiservercel.NewDeclField("namespaceName", apiservercel.StringType, true, nil, nil),
		"workflowRunName": apiservercel.NewDeclField("workflowRunName", apiservercel.StringType, true, nil, nil),
		"namespace":       apiservercel.NewDeclField("namespace", apiservercel.StringType, true, nil, nil),
		"labels":          apiservercel.NewDeclField("labels", apiservercel.NewMapType(apiservercel.StringType, apiservercel.StringType, 1000), true, nil, nil),
	})
	workflowplaneType := apiservercel.NewObjectType("WorkflowPlaneData", map[string]*apiservercel.DeclField{
		"secretStore": apiservercel.NewDeclField("secretStore", apiservercel.StringType, true, nil, nil),
	})

	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")

	provider := apiservercel.NewDeclTypeProvider(paramType, metadataType, workflowplaneType)
	providerOpts, err := provider.EnvOptions(baseEnv.CELTypeProvider())
	if err != nil {
		return nil, err
	}

	varOpts := make([]cel.EnvOption, 0, 4+len(providerOpts))
	varOpts = append(varOpts,
		cel.Variable(fieldParameters, paramType.CelType()),
		cel.Variable("metadata", metadataType.CelType()),
		cel.Variable("workflowplane", workflowplaneType.CelType()),
		cel.Variable("externalRefs", cel.DynType),
	)
	varOpts = append(varOpts, providerOpts...)

	return baseEnv.Extend(varOpts...)
}

// WithApplied returns an env with "applied" in scope as map<string,
// AppliedEntry>. AppliedEntry.status is Dyn-typed because operators populate
// the schema beneath it freely. Whether an applied.<id> reference matches a
// declared resources[].id is a separate AST-walk concern for the caller.
// Mirrors internal/validation/resource/cel_env.go's extendEnvWithApplied.
func WithApplied(env *cel.Env) (*cel.Env, error) {
	appliedEntryType := apiservercel.NewObjectType("AppliedEntry", map[string]*apiservercel.DeclField{
		"status": apiservercel.NewDeclField("status", apiservercel.DynType, true, nil, nil),
	})

	// Only the named AppliedEntry value type goes through the DeclTypeProvider.
	// The map wrapper is built inline at the cel.Type level — registering a
	// fresh apiservercel.MapType against an env that already carries
	// map<string, string> (metadata.labels / metadata.annotations) would hit
	// "type map definition differs between CEL environment and type provider"
	// because the provider tries to redefine the unnamed map type.
	provider := apiservercel.NewDeclTypeProvider(appliedEntryType)
	providerOpts, err := provider.EnvOptions(env.CELTypeProvider())
	if err != nil {
		return nil, fmt.Errorf("create applied type provider: %w", err)
	}

	opts := make([]cel.EnvOption, 0, 1+len(providerOpts))
	opts = append(opts, cel.Variable("applied", cel.MapType(cel.StringType, appliedEntryType.CelType())))
	opts = append(opts, providerOpts...)

	return env.Extend(opts...)
}

// buildCELEnv creates a schema-aware CEL environment with the given context fields and schema options.
// Schema-based fields (parameters, environmentConfigs) are derived from the provided schemas.
// Reflection-based fields (metadata, workload, etc.) come from the contextFields slice.
func buildCELEnv(contextFields []decltype.FieldInfo, opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	baseEnv, err := createBaseEnv()
	if err != nil {
		return nil, nil, err
	}

	return buildSchemaEnvironment(baseEnv, contextFields, opts)
}

// NewProjectEnv reflects the production project context, without component-only macros.
func NewProjectEnv(opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	base, err := cel.NewEnv(template.BaseCELExtensions()...)
	if err != nil {
		return nil, nil, err
	}
	fields := decltype.ExtractFields(reflect.TypeFor[projectpipeline.BaseContext](), schemaBasedFields)
	return buildSchemaEnvironment(base, fields, opts)
}

func buildSchemaEnvironment(baseEnv *cel.Env, contextFields []decltype.FieldInfo, opts SchemaOptions) (*cel.Env, *apiservercel.DeclTypeProvider, error) {
	numFields := len(contextFields) + len(schemaBasedFields)
	declTypes := make([]*apiservercel.DeclType, 0, numFields)
	varOpts := make([]cel.EnvOption, 0, numFields)

	// Register schema-based fields
	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")
	declTypes = append(declTypes, paramType)
	varOpts = append(varOpts, cel.Variable(fieldParameters, paramType.CelType()))

	environmentConfigsType := schemaToTypeOrEmpty(opts.EnvironmentConfigsSchema, "EnvironmentConfigs")
	declTypes = append(declTypes, environmentConfigsType)
	varOpts = append(varOpts, cel.Variable(fieldEnvironmentConfigs, environmentConfigsType.CelType()))

	// Register reflection-based fields
	for _, f := range contextFields {
		declTypes = append(declTypes, f.DeclType)
		varOpts = append(varOpts, cel.Variable(f.Name, f.DeclType.CelType()))
	}

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

// createBaseEnv creates the base CEL environment with the standard template
// extensions plus the component receiver macros (configurations.*,
// dependencies.*, workload.toServicePorts, ...). Used by the component and
// trait environments.
func createBaseEnv() (*cel.Env, error) {
	baseEnvOpts := template.BaseCELExtensions()
	baseEnvOpts = append(baseEnvOpts, context.CELExtensions()...)
	return cel.NewEnv(baseEnvOpts...)
}

// createResourceBaseEnv creates the base CEL environment for ResourceType
// templates. The component receiver macros are deliberately absent here — they
// are component-render-time concerns with no meaning for resource templates.
// Mirrors internal/validation/resource/cel_env.go.
func createResourceBaseEnv() (*cel.Env, error) {
	return cel.NewEnv(template.BaseCELExtensions()...)
}

// schemaToTypeOrEmpty converts a structural schema to a DeclType,
// returning an empty object type if schema is nil or conversion fails.
func schemaToTypeOrEmpty(schema *apiextschema.Structural, typeName string) *apiservercel.DeclType {
	if schema != nil {
		normalized := normalizeForCEL(schema)
		if dt := model.SchemaDeclType(normalized, false); dt != nil {
			return dt.MaybeAssignTypeName(typeName)
		}
	}
	return apiservercel.NewObjectType(typeName, map[string]*apiservercel.DeclField{})
}

// normalizeForCEL returns a shallow-cloned structural schema where nodes that
// have no top-level type but carry oneOf/anyOf/allOf variants are marked as
// x-kubernetes-int-or-string. This makes the Kubernetes SchemaDeclType
// function treat them as CEL dyn values instead of returning nil and
// silently dropping the enclosing field from the CEL type environment.
func normalizeForCEL(s *apiextschema.Structural) *apiextschema.Structural {
	if s == nil {
		return nil
	}
	out := *s

	if out.Type == "" && hasCompositionValidation(out.ValueValidation) {
		out.Extensions.XIntOrString = true
	}

	if out.Items != nil {
		out.Items = normalizeForCEL(out.Items)
	}

	if len(out.Properties) > 0 {
		props := make(map[string]apiextschema.Structural, len(out.Properties))
		for k, v := range out.Properties {
			normalized := normalizeForCEL(&v)
			props[k] = *normalized
		}
		out.Properties = props
	}

	if out.AdditionalProperties != nil && out.AdditionalProperties.Structural != nil {
		ap := *out.AdditionalProperties
		ap.Structural = normalizeForCEL(ap.Structural)
		out.AdditionalProperties = &ap
	}

	return &out
}

func hasCompositionValidation(v *apiextschema.ValueValidation) bool {
	return v != nil && (len(v.OneOf) > 0 || len(v.AnyOf) > 0 || len(v.AllOf) > 0)
}
