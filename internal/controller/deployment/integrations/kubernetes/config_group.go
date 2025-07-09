// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"path/filepath"
	"regexp"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

// This file contains the helper functions that are related to deploying configuration groups in Kubernetes.

// findConfigGroupValueForEnv returns the configuration value for the given environment
// from the given configuration value list. Returns nil if no matching configuration value is found.
func findConfigGroupValueForEnv(value []openchoreov1alpha1.ConfigurationValue,
	envGroup []openchoreov1alpha1.EnvironmentGroup, env *openchoreov1alpha1.Environment) *openchoreov1alpha1.ConfigurationValue {
	for _, v := range value {
		envName := controller.GetName(env)
		if v.Environment == envName {
			return &v
		}
		// If the environment group reference is set,
		// find if there is a value for the environment in the environment group
		for _, eg := range envGroup {
			if eg.Name == v.EnvironmentGroupRef {
				for _, ege := range eg.Environments {
					if ege == envName {
						return &v
					}
				}
			}
		}
	}
	return nil
}

var invalidEnvKeyChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeEnvVarKey(key string) string {
	// Replace invalid characters with underscore.
	newKey := strings.ToUpper(invalidEnvKeyChars.ReplaceAllString(key, "_"))

	// If the result is empty, return a default key.
	if len(newKey) == 0 {
		return ""
	}

	// Ensure the first character is a letter or an underscore.
	// Example: "123" -> "_123"
	first := newKey[0]
	if !(first >= 'A' && first <= 'Z' || first == '_') {
		newKey = "_" + newKey
	}
	return newKey
}

// mappedEnvVarConfig stores the filtered configuration values (plain text and secret) that are mapped
// to the environment variables in the deployable artifact for a single configuration group.
type mappedEnvVarConfig struct {
	PlainConfigs  []envVarConfig
	SecretConfigs []envVarConfig
}

// envVarConfig stores an individual plain text configuration value
// mapped in the deployable artifact to a configuration group.
type envVarConfig struct {
	EnvVarKey      string
	ConfigGroupKey string
}

// newMappedEnvVarConfig creates a new mappedEnvVarConfig instance for the given configuration group that maps
// the configuration group keys to the environment variables keys.
func newMappedEnvVarConfig(deployCtx *dataplane.DeploymentContext, cg *openchoreov1alpha1.ConfigurationGroup) *mappedEnvVarConfig {
	// Create a mapping of a key to env specific value for fast lookup
	// The map value can be nil if the configuration value is not found for the environment
	cgKeyValueMapping := make(map[string]*openchoreov1alpha1.ConfigurationValue)
	for _, cgConfig := range cg.Spec.Configurations {
		cgKeyValueMapping[cgConfig.Key] = findConfigGroupValueForEnv(cgConfig.Values,
			cg.Spec.EnvironmentGroups, deployCtx.Environment)
	}

	appCfg := deployCtx.DeployableArtifact.Spec.Configuration.Application

	plainConfigs := make([]envVarConfig, 0)
	secretConfigs := make([]envVarConfig, 0)

	// Find individual configuration group key mappings in the Env section
	// Example Configuration group mapping:
	// env:
	//   - key: REDIS_HOST
	//	   valueFrom:
	//	     configurationGroupRef:
	//		   name: redis-config
	//		   key: redis-host
	for _, ev := range appCfg.Env {
		if ev.Key == "" {
			continue
		}
		if ev.ValueFrom == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef == nil {
			continue
		}
		if ev.ValueFrom.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		cgRef := ev.ValueFrom.ConfigurationGroupRef
		// Find the matching configuration value for the key if it exists
		cgValue, ok := cgKeyValueMapping[cgRef.Key]
		if !ok || cgValue == nil {
			// This can happen in following cases:
			// 1. Deployable artifact is trying to map a configuration key that does not exist in the configuration group.
			// 2. Configuration value is not set for the environment in the configuration group.
			continue
		}

		if cgValue.Value != "" {
			p := envVarConfig{
				EnvVarKey:      ev.Key,
				ConfigGroupKey: cgRef.Key,
			}
			plainConfigs = append(plainConfigs, p)
		} else if cgValue.VaultKey != "" {
			s := envVarConfig{
				EnvVarKey:      ev.Key,
				ConfigGroupKey: cgRef.Key,
			}
			secretConfigs = append(secretConfigs, s)
		}
	}

	// Find bulk configuration group mappings in the EnvFrom section
	// Example Configuration group injection:
	// envFrom:
	//   - configurationGroupRef:
	//       name: redis-config
	for _, evf := range appCfg.EnvFrom {
		if evf.ConfigurationGroupRef == nil {
			continue
		}
		if evf.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		// Add all the keys from the configuration group
		for key, value := range cgKeyValueMapping {
			// Value is not set for the environment in the configuration group
			if value == nil {
				continue
			}
			if value.Value != "" {
				p := envVarConfig{
					EnvVarKey:      sanitizeEnvVarKey(key),
					ConfigGroupKey: key,
				}
				plainConfigs = append(plainConfigs, p)
			} else if value.VaultKey != "" {
				s := envVarConfig{
					EnvVarKey:      sanitizeEnvVarKey(key),
					ConfigGroupKey: key,
				}
				secretConfigs = append(secretConfigs, s)
			}
		}
	}

	return &mappedEnvVarConfig{
		PlainConfigs:  plainConfigs,
		SecretConfigs: secretConfigs,
	}
}

var invalidFileNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`) // keep ASCII letters, digits, dot, dash, underscore

// sanitizeFileName converts an arbitrary string into a filename that is
// • legal on Linux/Unix filesystems
// • portable across most other OSes
// • no longer than 255 bytes (ext4 / POSIX‐max component length)
func sanitizeFileName(name string) string {
	if name == "" {
		return "file"
	}

	// 1. Replace path separators up-front so the regex doesn’t leave them intact.
	name = strings.ReplaceAll(name, "/", "_")

	// 2. Replace anything outside our allow-list with an underscore.
	safe := invalidFileNameChars.ReplaceAllString(name, "_")

	// 3. Collapse runs of consecutive underscores produced by replacements.
	safe = strings.ReplaceAll(safe, "__", "_")
	for strings.Contains(safe, "__") { // quick iterative collapse
		safe = strings.ReplaceAll(safe, "__", "_")
	}

	// 4. Disallow bare “.” or “..” (current / parent dir).
	if safe == "." || safe == ".." {
		safe = "_" + safe
	}

	// 5. Ensure we didn’t end up with an empty string.
	if len(safe) == 0 {
		safe = "file"
	}

	// 6. Truncate to 255 bytes (UTF-8 safe because regex kept only single-byte runes).
	if len(safe) > 255 {
		safe = safe[:255]
	}

	return safe
}

// mappedFileMountConfig stores the filtered configuration values (plain text and secret) that are mapped
// to the file paths in the deployable artifact for a single configuration group.
type mappedFileMountConfig struct {
	PlainConfigs  []fileMountConfig
	SecretConfigs []fileMountConfig
}

// fileMountConfig stores an individual secret configuration value
// mapped in the deployable artifact to a configuration group.
type fileMountConfig struct {
	MountPath      string
	ConfigGroupKey string
}

// newMappedFileMountConfig creates a new mappedFileMountConfig instance for the given configuration group that maps
// the configuration group keys to the file mount paths.
func newMappedFileMountConfig(deployCtx *dataplane.DeploymentContext, cg *openchoreov1alpha1.ConfigurationGroup) *mappedFileMountConfig {
	// Create a mapping of a key to env specific value for fast lookup
	// The map value can be nil if the configuration value is not found for the environment
	cgKeyValueMapping := make(map[string]*openchoreov1alpha1.ConfigurationValue)
	for _, cgConfig := range cg.Spec.Configurations {
		cgKeyValueMapping[cgConfig.Key] = findConfigGroupValueForEnv(cgConfig.Values,
			cg.Spec.EnvironmentGroups, deployCtx.Environment)
	}

	appCfg := deployCtx.DeployableArtifact.Spec.Configuration.Application

	plainConfigs := make([]fileMountConfig, 0)
	secretConfigs := make([]fileMountConfig, 0)

	// Find individual configuration group key mappings in the FileMount section
	// Example Configuration group mapping:
	// fileMounts:
	//   - mountPath: /etc/certificates/ca.crt
	//	   valueFrom:
	//	     configurationGroupRef:
	//		   name: app-cert-config
	//		   key: ca-cert
	for _, fm := range appCfg.FileMounts {
		if fm.MountPath == "" {
			continue
		}
		if fm.ValueFrom == nil {
			continue
		}
		if fm.ValueFrom.ConfigurationGroupRef == nil {
			continue
		}
		if fm.ValueFrom.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		cgRef := fm.ValueFrom.ConfigurationGroupRef
		// Find the matching configuration value for the key if it exists
		cgValue, ok := cgKeyValueMapping[cgRef.Key]
		if !ok || cgValue == nil {
			// This can happen in following cases:
			// 1. Deployable artifact is trying to map a configuration key that does not exist in the configuration group.
			// 2. Configuration value is not set for the environment in the configuration group.
			continue
		}

		if cgValue.Value != "" {
			p := fileMountConfig{
				MountPath:      fm.MountPath,
				ConfigGroupKey: cgRef.Key,
			}
			plainConfigs = append(plainConfigs, p)
		} else if cgValue.VaultKey != "" {
			s := fileMountConfig{
				MountPath:      fm.MountPath,
				ConfigGroupKey: cgRef.Key,
			}
			secretConfigs = append(secretConfigs, s)
		}
	}

	// Find bulk configuration group mappings in the FileMountsFrom section
	// Example Configuration group injection:
	// fileMountsFrom:
	//   - configurationGroupRef:
	//       mountPath: /etc/certificates
	//       name: app-config
	for _, evf := range appCfg.FileMountsFrom {
		if evf.ConfigurationGroupRef == nil {
			continue
		}
		if evf.ConfigurationGroupRef.Name != controller.GetName(cg) {
			continue
		}
		// Add all the keys from the configuration group
		for key, value := range cgKeyValueMapping {
			// Value is not set for the environment in the configuration group
			if value == nil {
				continue
			}
			mountPath := filepath.Clean(filepath.Join(evf.ConfigurationGroupRef.MountPath, sanitizeFileName(key)))
			if value.Value != "" {
				p := fileMountConfig{
					MountPath:      mountPath,
					ConfigGroupKey: key,
				}
				plainConfigs = append(plainConfigs, p)
			} else if value.VaultKey != "" {
				s := fileMountConfig{
					MountPath:      mountPath,
					ConfigGroupKey: key,
				}
				secretConfigs = append(secretConfigs, s)
			}
		}
	}

	return &mappedFileMountConfig{
		PlainConfigs:  plainConfigs,
		SecretConfigs: secretConfigs,
	}
}
