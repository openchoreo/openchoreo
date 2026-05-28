// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

const (
	protocolTCP = "TCP"
	protocolUDP = "UDP"
)

// DerivedContext holds precomputed derived views of configurations, workload,
// and dependency data. CEL macros rewrite helper method calls (e.g.
// configurations.toConfigFileList()) to field selects on this struct
// (e.g. derived.configFileList), eliminating runtime map[string]any traversal.
type DerivedContext struct {
	ConfigFileList     []ConfigFileListEntry  `json:"configFileList"`
	SecretFileList     []SecretFileListEntry  `json:"secretFileList"`
	ContainerEnvFrom   []EnvFromEntry         `json:"containerEnvFrom"`
	ConfigVolumeMounts []VolumeMountEntry     `json:"configVolumeMounts"`
	ConfigVolumes      []VolumeEntry          `json:"configVolumes"`
	ConfigEnvs         []EnvsByContainerEntry `json:"configEnvs"`
	SecretEnvs         []EnvsByContainerEntry `json:"secretEnvs"`

	ServicePorts []ServicePortEntry `json:"servicePorts"`

	DependencyEnvVars      []EnvVarEntry      `json:"dependencyEnvVars"`
	DependencyVolumeMounts []VolumeMountEntry `json:"dependencyVolumeMounts"`
	DependencyVolumes      []VolumeEntry      `json:"dependencyVolumes"`
}

// BuildDerivedContext precomputes all derived views from typed inputs.
func BuildDerivedContext(
	configurations ContainerConfigurations,
	workload WorkloadData,
	dependencies ConnectionsContextData,
	prefix string,
) DerivedContext {
	return DerivedContext{
		ConfigFileList:         buildConfigFileList(configurations, prefix),
		SecretFileList:         buildSecretFileList(configurations, prefix),
		ContainerEnvFrom:       buildEnvFrom(configurations, prefix),
		ConfigVolumeMounts:     buildVolumeMounts(configurations),
		ConfigVolumes:          buildVolumes(configurations, prefix),
		ConfigEnvs:             buildConfigEnvs(configurations, prefix),
		SecretEnvs:             buildSecretEnvs(configurations, prefix),
		ServicePorts:           buildServicePorts(workload),
		DependencyEnvVars:      nonNilEnvVars(dependencies.EnvVars),
		DependencyVolumeMounts: nonNilVolumeMounts(dependencies.VolumeMounts),
		DependencyVolumes:      nonNilVolumes(dependencies.Volumes),
	}
}

func buildConfigFileList(cc ContainerConfigurations, prefix string) []ConfigFileListEntry {
	files := cc.Configs.Files
	result := make([]ConfigFileListEntry, len(files))
	for i, f := range files {
		result[i] = ConfigFileListEntry{
			Name:         f.Name,
			MountPath:    f.MountPath,
			Value:        f.Value,
			ResourceName: generateConfigResourceName(prefix, f.Name),
			RemoteRef:    f.RemoteRef,
		}
	}
	return result
}

func buildSecretFileList(cc ContainerConfigurations, prefix string) []SecretFileListEntry {
	files := cc.Secrets.Files
	result := make([]SecretFileListEntry, len(files))
	for i, f := range files {
		result[i] = SecretFileListEntry{
			Name:         f.Name,
			MountPath:    f.MountPath,
			ResourceName: generateSecretResourceName(prefix, f.Name, f.RemoteRef),
			RemoteRef:    f.RemoteRef,
		}
	}
	return result
}

func buildEnvFrom(cc ContainerConfigurations, prefix string) []EnvFromEntry {
	result := make([]EnvFromEntry, 0, 2)
	if len(cc.Configs.Envs) > 0 {
		result = append(result, EnvFromEntry{
			ConfigMapRef: &NameRef{Name: generateEnvResourceName(prefix)},
		})
	}
	if len(cc.Secrets.Envs) > 0 {
		result = append(result, EnvFromEntry{
			SecretRef: &NameRef{Name: generateSecretEnvResourceName(prefix, cc.Secrets.Envs)},
		})
	}
	return result
}

func buildVolumeMounts(cc ContainerConfigurations) []VolumeMountEntry {
	result := make([]VolumeMountEntry, 0, len(cc.Configs.Files)+len(cc.Secrets.Files))
	for _, f := range cc.Configs.Files {
		result = append(result, VolumeMountEntry{
			Name:      "file-mount-" + generateVolumeHash(f.MountPath, f.Name),
			MountPath: f.MountPath + "/" + f.Name,
			SubPath:   f.Name,
		})
	}
	for _, f := range cc.Secrets.Files {
		result = append(result, VolumeMountEntry{
			Name:      "file-mount-" + generateVolumeHash(f.MountPath, f.Name),
			MountPath: f.MountPath + "/" + f.Name,
			SubPath:   f.Name,
		})
	}
	return result
}

func buildVolumes(cc ContainerConfigurations, prefix string) []VolumeEntry {
	byName := make(map[string]VolumeEntry)
	for _, f := range cc.Configs.Files {
		name := "file-mount-" + generateVolumeHash(f.MountPath, f.Name)
		byName[name] = VolumeEntry{
			Name:      name,
			ConfigMap: &ConfigMapVolume{Name: generateConfigResourceName(prefix, f.Name)},
		}
	}
	for _, f := range cc.Secrets.Files {
		name := "file-mount-" + generateVolumeHash(f.MountPath, f.Name)
		byName[name] = VolumeEntry{
			Name:   name,
			Secret: &SecretVolume{SecretName: generateSecretResourceName(prefix, f.Name, f.RemoteRef)},
		}
	}

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	result := make([]VolumeEntry, len(names))
	for i, n := range names {
		result[i] = byName[n]
	}
	return result
}

func buildConfigEnvs(cc ContainerConfigurations, prefix string) []EnvsByContainerEntry {
	if len(cc.Configs.Envs) == 0 {
		return []EnvsByContainerEntry{}
	}
	return []EnvsByContainerEntry{{
		ResourceName: generateEnvResourceName(prefix),
		Envs:         cc.Configs.Envs,
	}}
}

func buildSecretEnvs(cc ContainerConfigurations, prefix string) []EnvsByContainerEntry {
	if len(cc.Secrets.Envs) == 0 {
		return []EnvsByContainerEntry{}
	}
	return []EnvsByContainerEntry{{
		ResourceName: generateSecretEnvResourceName(prefix, cc.Secrets.Envs),
		Envs:         cc.Secrets.Envs,
	}}
}

func buildServicePorts(workload WorkloadData) []ServicePortEntry {
	if len(workload.Endpoints) == 0 {
		return []ServicePortEntry{}
	}

	endpointNames := make([]string, 0, len(workload.Endpoints))
	for name := range workload.Endpoints {
		endpointNames = append(endpointNames, name)
	}
	sort.Strings(endpointNames)

	result := make([]ServicePortEntry, 0, len(workload.Endpoints))
	usedNames := make(map[string]bool)
	seenPortProto := make(map[string]bool)

	for _, epName := range endpointNames {
		ep := workload.Endpoints[epName]
		port := int64(ep.Port)
		targetPort := int64(ep.TargetPort)
		if targetPort == 0 {
			targetPort = port
		}
		protocol := mapEndpointTypeToProtocol(ep.Type)

		portProtoKey := fmt.Sprintf("%d/%s", port, protocol)
		if seenPortProto[portProtoKey] {
			continue
		}
		seenPortProto[portProtoKey] = true

		finalName := uniquePortName(epName, port, usedNames)
		usedNames[finalName] = true

		result = append(result, ServicePortEntry{
			Name:       finalName,
			Port:       port,
			TargetPort: targetPort,
			Protocol:   protocol,
		})
	}
	return result
}

func uniquePortName(endpointName string, port int64, usedNames map[string]bool) string {
	sanitized := sanitizePortName(endpointName)
	if sanitized == "" {
		sanitized = fmt.Sprintf("port-%d", port)
	}

	name := sanitized
	counter := 2
	for usedNames[name] {
		name = fmt.Sprintf("%s-%d", sanitized, counter)
		if len(name) > 15 {
			maxBase := 15 - len(fmt.Sprintf("-%d", counter))
			if maxBase > 0 {
				name = fmt.Sprintf("%s-%d", sanitized[:maxBase], counter)
			} else {
				name = fmt.Sprintf("p-%d", counter)
			}
		}
		counter++
	}
	return name
}

func nonNilEnvVars(s []EnvVarEntry) []EnvVarEntry {
	if s == nil {
		return []EnvVarEntry{}
	}
	return s
}

func nonNilVolumeMounts(s []VolumeMountEntry) []VolumeMountEntry {
	if s == nil {
		return []VolumeMountEntry{}
	}
	return s
}

func nonNilVolumes(s []VolumeEntry) []VolumeEntry {
	if s == nil {
		return []VolumeEntry{}
	}
	return s
}

func sanitizePortName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")

	var result strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			result.WriteRune(ch)
		}
	}
	sanitized := strings.Trim(result.String(), "-")

	if len(sanitized) == 0 {
		return ""
	}
	if len(sanitized) > 15 {
		sanitized = strings.TrimRight(sanitized[:15], "-")
	}
	return sanitized
}

func generateVolumeHash(mountPath, filename string) string {
	input := mountPath + "/" + filename
	h := fnv.New32a()
	h.Write([]byte(input))
	return fmt.Sprintf("%08x", h.Sum32())
}

func generateConfigResourceName(prefix, filename string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		"config",
		strings.ReplaceAll(filename, ".", "-"),
	)
}

// generateSecretResourceName generates a resource name for a secret file.
// The remoteRef content is included in the hash computation so that a change
// in secret reference produces a new ExternalSecret (and K8s Secret),
// preventing pods from reading stale values.
func generateSecretResourceName(prefix, filename string, remoteRef *RemoteRefData) string {
	return kubernetes.GenerateK8sNameWithExtraHashInput(
		kubernetes.MaxResourceNameLength,
		remoteRefContentHash(remoteRef),
		prefix,
		"secret",
		strings.ReplaceAll(filename, ".", "-"),
	)
}

// generateSecretEnvResourceName generates a resource name for secret env resources.
// The remoteRef content from all env entries is included in the hash computation
// so that a change in secret references produces a new ExternalSecret.
func generateSecretEnvResourceName(prefix string, envs []EnvConfiguration) string {
	return kubernetes.GenerateK8sNameWithExtraHashInput(
		kubernetes.MaxResourceNameLength,
		secretEnvsContentHash(envs),
		prefix,
		"env-secrets",
	)
}

func remoteRefContentHash(ref *RemoteRefData) string {
	if ref == nil {
		return ""
	}
	h := fnv.New32a()
	fmt.Fprintf(h, "%s\x00%s\x00%s", ref.Key, ref.Property, ref.Version)
	return fmt.Sprintf("%08x", h.Sum32())
}

func secretEnvsContentHash(envs []EnvConfiguration) string {
	if len(envs) == 0 {
		return ""
	}
	h := fnv.New32a()
	for _, env := range envs {
		if env.RemoteRef == nil {
			continue
		}
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00", env.Name, env.RemoteRef.Key, env.RemoteRef.Property, env.RemoteRef.Version)
	}
	return fmt.Sprintf("%08x", h.Sum32())
}

func generateEnvResourceName(prefix string) string {
	return kubernetes.GenerateK8sNameWithLengthLimit(
		kubernetes.MaxResourceNameLength,
		prefix,
		"env-configs",
	)
}

func mapEndpointTypeToProtocol(endpointType string) string {
	switch endpointType {
	case protocolTCP:
		return protocolTCP
	case protocolUDP:
		return protocolUDP
	default:
		return protocolTCP
	}
}
