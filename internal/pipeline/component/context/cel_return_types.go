// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"reflect"
	"strings"

	"github.com/google/cel-go/cel"
)

// Return type structs for CEL helper functions.
// These mirror the map[string]any shapes returned at runtime and are used
// by the validation environment to provide typed function declarations so
// that CEL's type checker can catch invalid field access in forEach loops.

// ConfigFileListEntry represents an element returned by configurations.toConfigFileList().
type ConfigFileListEntry struct {
	Name         string         `json:"name"`
	MountPath    string         `json:"mountPath"`
	Value        string         `json:"value"`
	ResourceName string         `json:"resourceName"`
	RemoteRef    *RemoteRefData `json:"remoteRef,omitempty"`
}

// SecretFileListEntry represents an element returned by configurations.toSecretFileList().
type SecretFileListEntry struct {
	Name         string         `json:"name"`
	MountPath    string         `json:"mountPath"`
	ResourceName string         `json:"resourceName"`
	RemoteRef    *RemoteRefData `json:"remoteRef,omitempty"`
}

// EnvFromEntry represents an element returned by configurations.toContainerEnvFrom().
type EnvFromEntry struct {
	ConfigMapRef *NameRef `json:"configMapRef,omitempty"`
	SecretRef    *NameRef `json:"secretRef,omitempty"`
}

// NameRef is a reference containing a name field.
type NameRef struct {
	Name string `json:"name"`
}

// VolumeMountEntry represents an element returned by configurations.toContainerVolumeMounts().
type VolumeMountEntry struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	SubPath   string `json:"subPath"`
}

// VolumeEntry represents an element returned by configurations.toVolumes().
type VolumeEntry struct {
	Name      string           `json:"name"`
	ConfigMap *ConfigMapVolume `json:"configMap,omitempty"`
	Secret    *SecretVolume    `json:"secret,omitempty"`
}

// ConfigMapVolume represents a configMap volume source.
type ConfigMapVolume struct {
	Name string `json:"name"`
}

// SecretVolume represents a secret volume source.
type SecretVolume struct {
	SecretName string `json:"secretName"`
}

// EnvsByContainerEntry represents an element returned by
// configurations.toConfigEnvsByContainer() or configurations.toSecretEnvsByContainer().
type EnvsByContainerEntry struct {
	ResourceName string             `json:"resourceName"`
	Envs         []EnvConfiguration `json:"envs"`
}

// ServicePortEntry represents an element returned by workload.toServicePorts().
type ServicePortEntry struct {
	Name       string `json:"name"`
	Port       int64  `json:"port"`
	TargetPort int64  `json:"targetPort"`
	Protocol   string `json:"protocol"`
}

// helperFuncDef defines a CEL helper function for the shared registry.
type helperFuncDef struct {
	funcName       string
	paramTypes     []*cel.Type
	returnGoType   reflect.Type
	runtimeBinding cel.OverloadOpt
}

// helperFuncRegistry is the single source of truth for all CEL helper functions.
// CELExtensions(), CELValidationExtensions(), and FunctionReturnTypes() all
// derive from this registry so the three cannot drift independently.
// When adding a new helper function, add an entry here.
var helperFuncRegistry = []helperFuncDef{
	{
		funcName:       "configurationsToConfigFileList",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[ConfigFileListEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToConfigFileListFunction),
	},
	{
		funcName:       "configurationsToSecretFileList",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[SecretFileListEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToSecretFileListFunction),
	},
	{
		funcName:       "configurationsToContainerEnvFrom",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[EnvFromEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToContainerEnvFromFunction),
	},
	{
		funcName:       "configurationsToContainerVolumeMounts",
		paramTypes:     []*cel.Type{cel.DynType},
		returnGoType:   reflect.TypeFor[VolumeMountEntry](),
		runtimeBinding: cel.UnaryBinding(configurationsToContainerVolumeMountsFunction),
	},
	{
		funcName:       "configurationsToVolumes",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[VolumeEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToVolumesFunction),
	},
	{
		funcName:       "configurationsToConfigEnvsByContainer",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[EnvsByContainerEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToConfigEnvsByContainerFunction),
	},
	{
		funcName:       "configurationsToSecretEnvsByContainer",
		paramTypes:     []*cel.Type{cel.DynType, cel.StringType},
		returnGoType:   reflect.TypeFor[EnvsByContainerEntry](),
		runtimeBinding: cel.BinaryBinding(configurationsToSecretEnvsByContainerFunction),
	},
	{
		funcName:       "workloadToServicePorts",
		paramTypes:     []*cel.Type{cel.DynType},
		returnGoType:   reflect.TypeFor[ServicePortEntry](),
		runtimeBinding: cel.UnaryBinding(workloadToServicePortsFunction),
	},
}

// helperMacros is the shared set of CEL macros registered by both
// CELExtensions() and CELValidationExtensions().
var helperMacros = []cel.Macro{
	toConfigFileListMacro, toSecretFileListMacro, toContainerEnvFromMacro,
	toContainerVolumeMountsMacro, toVolumesMacro, toConfigEnvsByContainerMacro,
	toSecretEnvsByContainerMacro, toServicePortsMacro, toContainerEnvMacro,
}

// FunctionReturnTypes returns the unique Go types used as return types by CEL helper functions.
// Used by the validation environment to register DeclTypes for field-access validation.
func FunctionReturnTypes() []reflect.Type {
	seen := make(map[reflect.Type]struct{})
	var types []reflect.Type
	for _, def := range helperFuncRegistry {
		if _, ok := seen[def.returnGoType]; !ok {
			seen[def.returnGoType] = struct{}{}
			types = append(types, def.returnGoType)
		}
	}
	return types
}

// CELExtensions returns CEL environment options for configuration helpers used by the runtime template engine.
// Function overloads return list(dyn) and include runtime bindings.
func CELExtensions() []cel.EnvOption {
	opts := make([]cel.EnvOption, 0, 1+len(helperFuncRegistry))
	opts = append(opts, cel.Macros(helperMacros...))
	for _, def := range helperFuncRegistry {
		overloadID := def.funcName + "_" + paramTypeSuffix(def.paramTypes)
		opts = append(opts, cel.Function(def.funcName,
			cel.Overload(overloadID, def.paramTypes, cel.ListType(cel.DynType), def.runtimeBinding),
		))
	}
	return opts
}

// CELValidationExtensions returns CEL environment options for the validation environment.
// Unlike CELExtensions(), function overloads declare typed return types instead of dyn,
// enabling the type checker to validate field access on forEach loop variables.
// No binding functions are registered since validation never evaluates expressions.
func CELValidationExtensions() []cel.EnvOption {
	opts := make([]cel.EnvOption, 0, 1+len(helperFuncRegistry))
	opts = append(opts, cel.Macros(helperMacros...))
	for _, def := range helperFuncRegistry {
		overloadID := def.funcName + "_" + paramTypeSuffix(def.paramTypes) + "_typed"
		typedReturn := cel.ListType(cel.ObjectType(def.returnGoType.Name()))
		opts = append(opts, cel.Function(def.funcName,
			cel.Overload(overloadID, def.paramTypes, typedReturn),
		))
	}
	return opts
}

// paramTypeSuffix generates an overload ID suffix from parameter types.
func paramTypeSuffix(paramTypes []*cel.Type) string {
	parts := make([]string, len(paramTypes))
	for i, pt := range paramTypes {
		parts[i] = celTypeShortName(pt)
	}
	return strings.Join(parts, "_")
}

// celTypeShortName returns a short identifier for a CEL type, used in overload IDs.
func celTypeShortName(t *cel.Type) string {
	switch {
	case t.IsEquivalentType(cel.DynType):
		return "dyn"
	case t.IsEquivalentType(cel.StringType):
		return "string"
	case t.IsEquivalentType(cel.IntType):
		return "int"
	case t.IsEquivalentType(cel.BoolType):
		return "bool"
	default:
		return t.String()
	}
}
