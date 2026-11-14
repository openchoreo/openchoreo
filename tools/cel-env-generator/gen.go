// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	apiservercel "k8s.io/apiserver/pkg/cel"

	componentctx "github.com/openchoreo/openchoreo/internal/pipeline/component/context"
	resourcepipeline "github.com/openchoreo/openchoreo/internal/pipeline/resource"
	"github.com/openchoreo/openchoreo/internal/validation/component/decltype"
)

const maxMapSize = 1000

type Var struct {
	Name     string          `json:"name"`
	CELType  string          `json:"celType,omitempty"`
	DeclType json.RawMessage `json:"declType,omitempty"`
}

type Ext struct {
	Base            []string `json:"base,omitempty"`
	CustomMacros    []string `json:"customMacros,omitempty"`
	CustomFunctions []string `json:"customFunctions,omitempty"`
	ContextMacros   []string `json:"contextMacros,omitempty"`
}

type EnvDesc struct {
	Kind       string `json:"kind"`
	Variables  []Var  `json:"variables"`
	HasApplied bool   `json:"hasApplied"`
	Extensions Ext    `json:"extensions"`
}

var schemaBased = map[string]bool{
	"parameters":         true,
	"environmentConfigs": true,
}

func generateForJob(targetDir string) error {
	// Component envs
	compFields := decltype.ExtractFields(reflect.TypeFor[componentctx.ComponentContext](), schemaBased)
	traitFields := decltype.ExtractFields(reflect.TypeFor[componentctx.TraitContext](), schemaBased)
	resFields := decltype.ExtractFields(reflect.TypeFor[resourcepipeline.BaseContext](), schemaBased)

	if err := writeEnv(targetDir, "ComponentType", compFields, false, componentExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "ClusterComponentType", compFields, false, componentExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "Trait", traitFields, false, componentExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "ClusterTrait", traitFields, false, componentExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "ResourceType", resFields, true, resourceExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "ClusterResourceType", resFields, true, resourceExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "Workflow", workflowVars(), false, workflowExt()); err != nil {
		return err
	}
	if err := writeEnv(targetDir, "ClusterWorkflow", workflowVars(), false, workflowExt()); err != nil {
		return err
	}
	return nil
}

func componentExt() Ext {
	return Ext{
		Base:            []string{"optionalTypes", "strings", "encoders", "math", "lists", "sets", "twoVarComprehensions"},
		CustomMacros:    []string{"oc_generate_name", "oc_dns_label", "oc_merge"},
		CustomFunctions: []string{"oc_omit"},
		ContextMacros: []string{
			"toConfigFileList",
			"toSecretFileList",
			"toContainerEnvFrom",
			"toContainerVolumeMounts",
			"toVolumes",
			"toConfigEnvsByContainer",
			"toSecretEnvsByContainer",
			"toServicePorts",
			"toContainerEnvs",
			"toEndpointResources",
		},
	}
}

func resourceExt() Ext {
	return Ext{
		Base:            []string{"optionalTypes", "strings", "encoders", "math", "lists", "sets", "twoVarComprehensions"},
		CustomMacros:    []string{"oc_generate_name", "oc_dns_label", "oc_merge"},
		CustomFunctions: []string{"oc_omit"},
		ContextMacros:   []string{},
	}
}

func workflowExt() Ext {
	return Ext{
		Base:            []string{"optionalTypes", "strings", "encoders", "math", "lists", "sets", "twoVarComprehensions"},
		CustomMacros:    []string{"oc_generate_name", "oc_dns_label", "oc_merge"},
		CustomFunctions: []string{"oc_omit"},
		ContextMacros:   []string{},
	}
}

func workflowVars() []decltype.FieldInfo {
	// Schema-based fields are handled separately. Include metadata, workflowplane,
	// and externalRefs markers for the loader.
	// Build decl types directly
	metadataType := apiservercel.NewObjectType("WorkflowMetadata", map[string]*apiservercel.DeclField{
		"namespaceName":   apiservercel.NewDeclField("namespaceName", apiservercel.StringType, true, nil, nil),
		"workflowRunName": apiservercel.NewDeclField("workflowRunName", apiservercel.StringType, true, nil, nil),
		"namespace":       apiservercel.NewDeclField("namespace", apiservercel.StringType, true, nil, nil),
		"labels": apiservercel.NewDeclField("labels",
			apiservercel.NewMapType(apiservercel.StringType, apiservercel.StringType, maxMapSize),
			true, nil, nil),
	})
	wfpType := apiservercel.NewObjectType("WorkflowPlaneData", map[string]*apiservercel.DeclField{
		"secretStore": apiservercel.NewDeclField("secretStore", apiservercel.StringType, true, nil, nil),
	})
	return []decltype.FieldInfo{
		{Name: "metadata", DeclType: metadataType},
		{Name: "workflowplane", DeclType: wfpType},
	}
}

func writeEnv(targetDir, kind string, fields []decltype.FieldInfo, hasApplied bool, ext Ext) error {
	declTypes := make([]*apiservercel.DeclType, 0, len(fields)+2)
	for _, f := range fields {
		declTypes = append(declTypes, f.DeclType)
	}
	provider := apiservercel.NewDeclTypeProvider(declTypes...)
	_ = provider

	var vars []Var
	// parameters/envconfigs added separately by schema
	vars = append(vars, Var{Name: "parameters", CELType: "object"})
	vars = append(vars, Var{Name: "environmentConfigs", CELType: "object"})
	for _, f := range fields {
		b, _ := json.Marshal(f.DeclType)
		vars = append(vars, Var{Name: f.Name, CELType: celTypeName(f.DeclType), DeclType: b})
	}

	if hasApplied {
		appliedType := apiservercel.NewMapType(apiservercel.StringType,
			apiservercel.NewObjectType("AppliedEntry", map[string]*apiservercel.DeclField{
				"status": apiservercel.NewDeclField("status", apiservercel.DynType, true, nil, nil),
			}), maxMapSize)
		b, _ := json.Marshal(appliedType)
		vars = append(vars, Var{Name: "applied", CELType: "map(string, AppliedEntry)", DeclType: b})
	}

	desc := EnvDesc{
		Kind:       kind,
		Variables:  vars,
		HasApplied: hasApplied,
		Extensions: ext,
	}
	path := filepath.Join(targetDir, fmt.Sprintf("%s_celenv.json", strings.ToLower(kind)))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(desc); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func celTypeName(dt *apiservercel.DeclType) string {
	if dt == nil {
		return "dyn"
	}
	return dt.CelType().String()
}
