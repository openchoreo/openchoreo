// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
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

// CELValidationExtensions returns CEL environment options for the validation environment.
// Unlike CELExtensions(), function overloads declare typed return types instead of dyn,
// enabling the type checker to validate field access on forEach loop variables.
// No binding functions are registered since validation never evaluates expressions.
func CELValidationExtensions() []cel.EnvOption {
	return []cel.EnvOption{
		// Same macros as the runtime environment
		cel.Macros(toConfigFileListMacro, toSecretFileListMacro, toContainerEnvFromMacro, toContainerVolumeMountsMacro, toVolumesMacro, toConfigEnvsByContainerMacro, toSecretEnvsByContainerMacro, toServicePortsMacro, toContainerEnvMacro),
		// Typed function declarations for validation
		cel.Function("configurationsToConfigFileList",
			cel.Overload("configurationsToConfigFileList_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("ConfigFileListEntry")),
			),
		),
		cel.Function("configurationsToSecretFileList",
			cel.Overload("configurationsToSecretFileList_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("SecretFileListEntry")),
			),
		),
		cel.Function("configurationsToContainerEnvFrom",
			cel.Overload("configurationsToContainerEnvFrom_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("EnvFromEntry")),
			),
		),
		cel.Function("configurationsToContainerVolumeMounts",
			cel.Overload("configurationsToContainerVolumeMounts_dyn_typed",
				[]*cel.Type{cel.DynType}, cel.ListType(cel.ObjectType("VolumeMountEntry")),
			),
		),
		cel.Function("configurationsToVolumes",
			cel.Overload("configurationsToVolumes_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("VolumeEntry")),
			),
		),
		cel.Function("configurationsToConfigEnvsByContainer",
			cel.Overload("configurationsToConfigEnvsByContainer_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("EnvsByContainerEntry")),
			),
		),
		cel.Function("configurationsToSecretEnvsByContainer",
			cel.Overload("configurationsToSecretEnvsByContainer_dyn_string_typed",
				[]*cel.Type{cel.DynType, cel.StringType}, cel.ListType(cel.ObjectType("EnvsByContainerEntry")),
			),
		),
		cel.Function("workloadToServicePorts",
			cel.Overload("workloadToServicePorts_dyn_typed",
				[]*cel.Type{cel.DynType}, cel.ListType(cel.ObjectType("ServicePortEntry")),
			),
		),
	}
}
