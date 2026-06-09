// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/openchoreo/openchoreo/api/v1alpha1"
)

// ResourceDependencyItem represents a single resource dependency with its target metadata
// and pre-computed env vars, volume mounts, and volumes for the consuming container.
// Mirrors ConnectionItem's role on the endpoint side.
type ResourceDependencyItem struct {
	Ref          string             `json:"ref"`
	EnvVars      []EnvVarEntry      `json:"envVars"`
	VolumeMounts []VolumeMountEntry `json:"volumeMounts"`
	Volumes      []VolumeEntry      `json:"volumes"`
}

// Sentinel errors returned by BuildResourceDependencyItem. Callers can use errors.Is to
// distinguish wait-for-resolve failures from permanent misconfigurations.
var (
	// ErrOutputNotResolved indicates a binding references an output that is not present in
	// the provider ResourceReleaseBinding's status.outputs[]. Transient: may resolve once
	// the provider catches up.
	ErrOutputNotResolved = errors.New("output not resolved on resource release binding")

	// ErrInvalidFileBinding indicates a fileBindings entry targets a value-kind output, which
	// has no underlying DP-side object to mount. Permanent misconfiguration in the workload
	// spec; will not resolve by waiting.
	ErrInvalidFileBinding = errors.New("output kind cannot be mounted as file")
)

// BuildResourceDependencyItem produces the env vars, volume mounts, and volumes that wire the
// outputs of a resolved ResourceReleaseBinding into a consuming container per the workload's
// dependency declaration. Pure function — no client, no logger, no controller-runtime imports.
//
// Dispatch per-binding:
//   - envBindings[outputName] → EnvVarEntry with {Value} (value-kind output)
//     or {ValueFrom.SecretKeyRef} / {ValueFrom.ConfigMapKeyRef} (ref-kind outputs)
//   - fileBindings[outputName] → VolumeMountEntry + VolumeEntry; output's source kind
//     must be SecretKeyRef or ConfigMapKeyRef (value-kind is rejected with ErrInvalidFileBinding)
//
// Volumes are deduped per (dep.Ref, secretName) and (dep.Ref, configMapName), so multiple file
// bindings that resolve to keys in the same Secret/ConfigMap share one volume with multiple
// mounts. Volume names are deterministic (FNV-1a 32-bit hash).
//
// Returns an error wrapping a sentinel (ErrOutputNotResolved / ErrInvalidFileBinding) on the
// first binding failure; the partial item is discarded. All-or-nothing per-dep semantics:
// callers should surface the error as a single PendingResourceDependency entry.
func BuildResourceDependencyItem(
	dep v1alpha1.WorkloadResourceDependency,
	outputs []v1alpha1.ResolvedResourceOutput,
) (ResourceDependencyItem, error) {
	item := ResourceDependencyItem{
		Ref:          dep.Ref,
		EnvVars:      []EnvVarEntry{},
		VolumeMounts: []VolumeMountEntry{},
		Volumes:      []VolumeEntry{},
	}

	outputByName := make(map[string]v1alpha1.ResolvedResourceOutput, len(outputs))
	for _, out := range outputs {
		outputByName[out.Name] = out
	}

	// Sorted output-name iteration so re-renders are byte-stable.
	envBindingNames := sortedKeys(dep.EnvBindings)
	fileBindingNames := sortedKeys(dep.FileBindings)

	for _, outputName := range envBindingNames {
		envVarName := dep.EnvBindings[outputName]
		out, ok := outputByName[outputName]
		if !ok {
			return ResourceDependencyItem{}, fmt.Errorf("%w: %s", ErrOutputNotResolved, outputName)
		}
		item.EnvVars = append(item.EnvVars, makeEnvVar(envVarName, out))
	}

	// Volume dedup keyed on (sourceKind, sourceName). One volume per unique source object;
	// multiple file bindings into the same source produce multiple mounts pointing at it.
	type volumeKey struct {
		kind string // "secret" | "configmap"
		name string
	}
	volumes := make(map[volumeKey]VolumeEntry)

	for _, outputName := range fileBindingNames {
		mountPath := dep.FileBindings[outputName]
		out, ok := outputByName[outputName]
		if !ok {
			return ResourceDependencyItem{}, fmt.Errorf("%w: %s", ErrOutputNotResolved, outputName)
		}

		switch {
		case out.SecretKeyRef != nil:
			key := volumeKey{kind: "secret", name: out.SecretKeyRef.Name}
			volName := resourceDepVolumeName(dep.Ref, key.kind, key.name)
			if _, exists := volumes[key]; !exists {
				volumes[key] = VolumeEntry{
					Name:   volName,
					Secret: &SecretVolume{SecretName: out.SecretKeyRef.Name},
				}
			}
			item.VolumeMounts = append(item.VolumeMounts, VolumeMountEntry{
				Name:      volName,
				MountPath: mountPath,
				SubPath:   out.SecretKeyRef.Key,
			})
		case out.ConfigMapKeyRef != nil:
			key := volumeKey{kind: "configmap", name: out.ConfigMapKeyRef.Name}
			volName := resourceDepVolumeName(dep.Ref, key.kind, key.name)
			if _, exists := volumes[key]; !exists {
				volumes[key] = VolumeEntry{
					Name:      volName,
					ConfigMap: &ConfigMapVolume{Name: out.ConfigMapKeyRef.Name},
				}
			}
			item.VolumeMounts = append(item.VolumeMounts, VolumeMountEntry{
				Name:      volName,
				MountPath: mountPath,
				SubPath:   out.ConfigMapKeyRef.Key,
			})
		default:
			// value-kind output: nothing to mount.
			return ResourceDependencyItem{}, fmt.Errorf("%w: %s", ErrInvalidFileBinding, outputName)
		}
	}

	// Emit volumes in sorted order for deterministic rendering.
	volumeKeys := make([]volumeKey, 0, len(volumes))
	for k := range volumes {
		volumeKeys = append(volumeKeys, k)
	}
	sort.Slice(volumeKeys, func(i, j int) bool {
		if volumeKeys[i].kind != volumeKeys[j].kind {
			return volumeKeys[i].kind < volumeKeys[j].kind
		}
		return volumeKeys[i].name < volumeKeys[j].name
	})
	for _, k := range volumeKeys {
		item.Volumes = append(item.Volumes, volumes[k])
	}

	return item, nil
}

// makeEnvVar dispatches on the output's source kind to produce a literal-value or valueFrom env var.
func makeEnvVar(envVarName string, out v1alpha1.ResolvedResourceOutput) EnvVarEntry {
	switch {
	case out.SecretKeyRef != nil:
		return EnvVarEntry{
			Name: envVarName,
			ValueFrom: &EnvVarSourceEntry{
				SecretKeyRef: &KeyRef{
					Name: out.SecretKeyRef.Name,
					Key:  out.SecretKeyRef.Key,
				},
			},
		}
	case out.ConfigMapKeyRef != nil:
		return EnvVarEntry{
			Name: envVarName,
			ValueFrom: &EnvVarSourceEntry{
				ConfigMapKeyRef: &KeyRef{
					Name: out.ConfigMapKeyRef.Name,
					Key:  out.ConfigMapKeyRef.Key,
				},
			},
		}
	default:
		// value-kind output (validated upstream as exactly-one).
		return EnvVarEntry{Name: envVarName, Value: out.Value}
	}
}

// resourceDepVolumeName produces a deterministic, length-bounded volume name for a resource
// dependency's mounted source. Pattern: r-{hash(ref:kind:name)}. FNV-1a 32-bit matches the
// existing generateVolumeHash convention used for configurations.toVolumes().
func resourceDepVolumeName(ref, sourceKind, sourceName string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ref))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(sourceKind))
	_, _ = h.Write([]byte{':'})
	_, _ = h.Write([]byte(sourceName))
	return fmt.Sprintf("r-%08x", h.Sum32())
}

// sortedKeys returns the keys of m in sorted order so callers iterate deterministically.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
