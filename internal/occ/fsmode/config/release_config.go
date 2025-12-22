// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// ReleaseConfig represents the release configuration file
type ReleaseConfig struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`

	// ComponentRelease output configuration
	ComponentReleaseDefaults *ComponentReleaseDefaults `yaml:"componentReleaseDefaults,omitempty"`

	// ReleaseBinding output configuration
	ReleaseBindingDefaults *ReleaseBindingDefaults `yaml:"releaseBindingDefaults,omitempty"`
}

// ComponentReleaseDefaults contains settings for ComponentRelease generation
type ComponentReleaseDefaults struct {
	DefaultOutputDir string                          `yaml:"defaultOutputDir,omitempty"`
	Projects         map[string]ProjectReleaseConfig `yaml:"projects,omitempty"`
}

// ProjectReleaseConfig contains project-level release configuration
type ProjectReleaseConfig struct {
	DefaultOutputDir string            `yaml:"defaultOutputDir,omitempty"`
	Components       map[string]string `yaml:"components,omitempty"` // component name -> output dir
}

// ReleaseBindingDefaults contains settings for ReleaseBinding generation
type ReleaseBindingDefaults struct {
	DefaultOutputDir string                          `yaml:"defaultOutputDir,omitempty"`
	Projects         map[string]ProjectBindingConfig `yaml:"projects,omitempty"`
}

// ProjectBindingConfig contains project-level binding configuration
type ProjectBindingConfig struct {
	DefaultOutputDir string            `yaml:"defaultOutputDir,omitempty"`
	Components       map[string]string `yaml:"components,omitempty"` // component name -> output dir
}

// LoadReleaseConfig loads a release config from file
func LoadReleaseConfig(path string) (*ReleaseConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ReleaseConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &config, nil
}

// GetReleaseOutputDir resolves the output directory for a component release
// Resolution priority:
// 1. componentReleaseDefaults.projects[projectName].components[componentName]
// 2. componentReleaseDefaults.projects[projectName].defaultOutputDir
// 3. componentReleaseDefaults.defaultOutputDir
// 4. empty string (caller should use their own default)
func (c *ReleaseConfig) GetReleaseOutputDir(projectName, componentName string) string {
	if c == nil || c.ComponentReleaseDefaults == nil {
		return ""
	}

	// Check project-specific config
	if projectConfig, ok := c.ComponentReleaseDefaults.Projects[projectName]; ok {
		// Check component-specific override
		if componentDir, ok := projectConfig.Components[componentName]; ok && componentDir != "" {
			return componentDir
		}
		// Check project default
		if projectConfig.DefaultOutputDir != "" {
			return projectConfig.DefaultOutputDir
		}
	}

	// Fall back to global default
	return c.ComponentReleaseDefaults.DefaultOutputDir
}

// GetBindingOutputDir resolves the output directory for a release binding
// Resolution priority:
// 1. releaseBindingDefaults.projects[projectName].components[componentName]
// 2. releaseBindingDefaults.projects[projectName].defaultOutputDir
// 3. releaseBindingDefaults.defaultOutputDir
// 4. empty string (caller should use their own default)
func (c *ReleaseConfig) GetBindingOutputDir(projectName, componentName string) string {
	if c == nil || c.ReleaseBindingDefaults == nil {
		return ""
	}

	// Check project-specific config
	if projectConfig, ok := c.ReleaseBindingDefaults.Projects[projectName]; ok {
		// Check component-specific override
		if componentDir, ok := projectConfig.Components[componentName]; ok && componentDir != "" {
			return componentDir
		}
		// Check project default
		if projectConfig.DefaultOutputDir != "" {
			return projectConfig.DefaultOutputDir
		}
	}

	// Fall back to global default
	return c.ReleaseBindingDefaults.DefaultOutputDir
}

// Validate validates the config structure
func (c *ReleaseConfig) Validate() error {
	if c.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if c.APIVersion != "openchoreo.dev/v1alpha1" {
		return fmt.Errorf("unsupported apiVersion: %s (expected openchoreo.dev/v1alpha1)", c.APIVersion)
	}

	if c.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if c.Kind != "ReleaseConfig" {
		return fmt.Errorf("unsupported kind: %s (expected ReleaseConfig)", c.Kind)
	}

	return nil
}
