// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package resource

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	apiextschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel/model"
	apiservercel "k8s.io/apiserver/pkg/cel"

	"github.com/openchoreo/openchoreo/internal/template"
	"github.com/openchoreo/openchoreo/internal/validation/component/decltype"
)

// schemaBasedFields are populated from user-provided schemas, not reflection.
// validationContext does not declare these fields; the skip set keeps
// ExtractFields behavior parallel with internal/validation/component.
var schemaBasedFields = map[string]bool{
	"parameters":         true,
	"environmentConfigs": true,
}

// validationContext mirrors the runtime CEL surface exposed by the resource
// pipeline (internal/pipeline/resource/pipeline.go::buildBaseContext).
// metadata.* and dataplane.* keys here must stay in sync with
// metadataContextToMap and dataPlaneContextToMap in the pipeline. This struct
// is reflection-only and is never instantiated.
type validationContext struct {
	Metadata  metadataView  `json:"metadata"`
	DataPlane dataplaneView `json:"dataplane"`
}

type metadataView struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	ResourceNamespace string            `json:"resourceNamespace"`
	ResourceName      string            `json:"resourceName"`
	ResourceUID       string            `json:"resourceUID"`
	ProjectName       string            `json:"projectName"`
	ProjectUID        string            `json:"projectUID"`
	EnvironmentName   string            `json:"environmentName"`
	EnvironmentUID    string            `json:"environmentUID"`
	DataPlaneName     string            `json:"dataPlaneName"`
	DataPlaneUID      string            `json:"dataPlaneUID"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
}

type dataplaneView struct {
	SecretStore           string                    `json:"secretStore"`
	ObservabilityPlaneRef observabilityPlaneRefView `json:"observabilityPlaneRef"`
}

type observabilityPlaneRefView struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

var resourceContextFields = decltype.ExtractFields(reflect.TypeFor[validationContext](), schemaBasedFields)

// SchemaOptions provides schema configuration for resource CEL environment building.
// Mirrors internal/validation/component.SchemaOptions; defined here to keep the
// resource validation package self-contained.
type SchemaOptions struct {
	ParametersSchema         *apiextschema.Structural
	EnvironmentConfigsSchema *apiextschema.Structural
}

// buildResourceCELEnv creates a schema-aware CEL environment for ResourceType
// and ClusterResourceType validation. The base env exposes metadata,
// parameters, environmentConfigs, and dataplane as top-level variables.
// applied.<id> is intentionally not in scope; extendEnvWithApplied layers it
// on for outputs and readyWhen validation.
func buildResourceCELEnv(opts SchemaOptions) (*cel.Env, error) {
	baseEnv, err := createBaseEnv()
	if err != nil {
		return nil, err
	}

	numFields := len(resourceContextFields) + len(schemaBasedFields)
	declTypes := make([]*apiservercel.DeclType, 0, numFields)
	varOpts := make([]cel.EnvOption, 0, numFields)

	paramType := schemaToTypeOrEmpty(opts.ParametersSchema, "Parameters")
	declTypes = append(declTypes, paramType)
	varOpts = append(varOpts, cel.Variable("parameters", paramType.CelType()))

	envConfigsType := schemaToTypeOrEmpty(opts.EnvironmentConfigsSchema, "EnvironmentConfigs")
	declTypes = append(declTypes, envConfigsType)
	varOpts = append(varOpts, cel.Variable("environmentConfigs", envConfigsType.CelType()))

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

// extendEnvWithApplied returns a new env with applied in scope as
// map<string, AppliedEntry>. AppliedEntry has a single Dyn-typed status field;
// the schema beneath status is not type-checked because operators populate it
// freely. Verifying that an applied.<id> reference matches a declared
// resources[].id is a separate AST-walk concern handled by the caller.
func extendEnvWithApplied(env *cel.Env) (*cel.Env, error) {
	appliedEntryType := apiservercel.NewObjectType("AppliedEntry", map[string]*apiservercel.DeclField{
		"status": apiservercel.NewDeclField("status", apiservercel.DynType, true, nil, nil),
	})

	// Only the named Object value type goes through the DeclTypeProvider.
	// The map wrapper is constructed inline at the cel.Type level — registering
	// a fresh apiservercel.MapType against an env that already carries
	// map<string, string> (from metadata.labels / metadata.annotations) would
	// trigger "type map definition differs between CEL environment and type
	// provider" because the provider tries to redefine the unnamed map type.
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

// createBaseEnv creates the base CEL environment with the standard
// template extensions (oc_omit and friends). Component-specific helpers
// (configurations.*, dependencies.*, connections.*) are deliberately absent —
// those are component-render-time concerns and have no meaning for resource
// templates.
func createBaseEnv() (*cel.Env, error) {
	return cel.NewEnv(template.BaseCELExtensions()...)
}

// schemaToTypeOrEmpty converts a structural schema to a DeclType, returning
// an empty object type if the schema is nil or unconvertible.
func schemaToTypeOrEmpty(schema *apiextschema.Structural, typeName string) *apiservercel.DeclType {
	if schema != nil {
		if dt := model.SchemaDeclType(schema, false); dt != nil {
			return dt.MaybeAssignTypeName(typeName)
		}
	}
	return apiservercel.NewObjectType(typeName, map[string]*apiservercel.DeclField{})
}
