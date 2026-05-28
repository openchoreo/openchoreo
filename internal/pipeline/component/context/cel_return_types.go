// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

// Type structs for DerivedContext fields. These define the shape of
// precomputed views accessible via CEL macros (e.g. derived.configFileList).
// The validation environment uses struct reflection on DerivedContext to
// register these types with the CEL type checker.

// ConfigFileListEntry represents an element of derived.configFileList.
type ConfigFileListEntry struct {
	Name         string         `json:"name"`
	MountPath    string         `json:"mountPath"`
	Value        string         `json:"value"`
	ResourceName string         `json:"resourceName"`
	RemoteRef    *RemoteRefData `json:"remoteRef,omitempty"`
}

// SecretFileListEntry represents an element of derived.secretFileList.
type SecretFileListEntry struct {
	Name         string         `json:"name"`
	MountPath    string         `json:"mountPath"`
	ResourceName string         `json:"resourceName"`
	RemoteRef    *RemoteRefData `json:"remoteRef,omitempty"`
}

// EnvFromEntry represents an element of derived.containerEnvFrom.
type EnvFromEntry struct {
	ConfigMapRef *NameRef `json:"configMapRef,omitempty"`
	SecretRef    *NameRef `json:"secretRef,omitempty"`
}

// NameRef is a reference containing a name field.
type NameRef struct {
	Name string `json:"name"`
}

// EnvVarEntry mirrors corev1.EnvVar for the subset of shapes the platform emits when
// merging endpoint connection env vars and resource dependency env vars into
// ${dependencies.envVars}. JSON-compatible with corev1.EnvVar so the rendered Pod spec is
// unchanged.
type EnvVarEntry struct {
	Name      string             `json:"name"`
	Value     string             `json:"value,omitempty"`
	ValueFrom *EnvVarSourceEntry `json:"valueFrom,omitempty"`
}

// EnvVarSourceEntry mirrors corev1.EnvVarSource for the two ref kinds the platform emits.
type EnvVarSourceEntry struct {
	SecretKeyRef    *KeyRef `json:"secretKeyRef,omitempty"`
	ConfigMapKeyRef *KeyRef `json:"configMapKeyRef,omitempty"`
}

// KeyRef references a single key within a Secret or ConfigMap.
type KeyRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// VolumeMountEntry represents an element of derived.containerVolumeMounts.
type VolumeMountEntry struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	SubPath   string `json:"subPath,omitempty"`
}

// VolumeEntry represents an element of derived.volumes.
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

// EnvsByContainerEntry represents an element of derived.configEnvs or derived.secretEnvs.
type EnvsByContainerEntry struct {
	ResourceName string             `json:"resourceName"`
	Envs         []EnvConfiguration `json:"envs"`
}

// ServicePortEntry represents an element of derived.servicePorts.
type ServicePortEntry struct {
	Name       string `json:"name"`
	Port       int64  `json:"port"`
	TargetPort int64  `json:"targetPort"`
	Protocol   string `json:"protocol"`
}
