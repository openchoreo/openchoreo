// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package context

import (
	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MergeWorkloadOverrides merges workload overrides into the base workload.
// Supports merging container env and file configurations.
func MergeWorkloadOverrides(baseWorkload *openchoreov1alpha1.Workload, overrides *openchoreov1alpha1.WorkloadOverrideTemplateSpec) *openchoreov1alpha1.Workload {
	if baseWorkload == nil {
		return nil
	}

	if overrides == nil || overrides.Container == nil {
		return baseWorkload
	}

	merged := baseWorkload.DeepCopy()
	merged.Spec.Container.Env = mergeEnvConfigs(merged.Spec.Container.Env, overrides.Container.Env)
	merged.Spec.Container.Files = mergeFileConfigs(merged.Spec.Container.Files, overrides.Container.Files)

	return merged
}

// mergeEnvConfigs merges environment variable configurations.
// Override env vars replace base env vars with the same key.
func mergeEnvConfigs(baseEnvs []openchoreov1alpha1.EnvVar, overrideEnvs []openchoreov1alpha1.EnvVar) []openchoreov1alpha1.EnvVar {
	if len(overrideEnvs) == 0 {
		return baseEnvs
	}

	overrideEnvMap := make(map[string]openchoreov1alpha1.EnvVar)
	for _, env := range overrideEnvs {
		overrideEnvMap[env.Key] = env
	}

	merged := make([]openchoreov1alpha1.EnvVar, 0, len(baseEnvs))
	for _, baseEnv := range baseEnvs {
		if overrideEnv, exists := overrideEnvMap[baseEnv.Key]; exists {
			merged = append(merged, overrideEnv)
			delete(overrideEnvMap, baseEnv.Key)
		} else {
			merged = append(merged, baseEnv)
		}
	}

	for _, env := range overrideEnvs {
		if _, exists := overrideEnvMap[env.Key]; exists {
			merged = append(merged, env)
		}
	}

	return merged
}

// mergeFileConfigs merges file configurations.
// Override files replace base files with the same key.
func mergeFileConfigs(baseFiles []openchoreov1alpha1.FileVar, overrideFiles []openchoreov1alpha1.FileVar) []openchoreov1alpha1.FileVar {
	if len(overrideFiles) == 0 {
		return baseFiles
	}

	overrideFileMap := make(map[string]openchoreov1alpha1.FileVar)
	for _, file := range overrideFiles {
		overrideFileMap[file.Key] = file
	}

	merged := make([]openchoreov1alpha1.FileVar, 0, len(baseFiles))
	for _, baseFile := range baseFiles {
		if overrideFile, exists := overrideFileMap[baseFile.Key]; exists {
			merged = append(merged, overrideFile)
			delete(overrideFileMap, baseFile.Key)
		} else {
			merged = append(merged, baseFile)
		}
	}

	for _, file := range overrideFiles {
		if _, exists := overrideFileMap[file.Key]; exists {
			merged = append(merged, file)
		}
	}

	return merged
}
