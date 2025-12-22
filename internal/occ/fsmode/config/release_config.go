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
	APIVersion       string                   `yaml:"apiVersion"`
	Kind             string                   `yaml:"kind"`
	DefaultOutputDir string                   `yaml:"defaultOutputDir,omitempty"`
	Projects         map[string]ProjectConfig `yaml:"projects,omitempty"`
}

// ProjectConfig represents project-specific release configuration
type ProjectConfig struct {
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

// GetOutputDir resolves the output directory for a component
// Resolution priority:
// 1. config.projects[projectName].components[componentName]
// 2. config.projects[projectName].defaultOutputDir
// 3. config.defaultOutputDir
// 4. empty string (caller should use their own default)
func (c *ReleaseConfig) GetOutputDir(projectName, componentName string) string {
	if c == nil {
		return ""
	}

	// Check project-specific config
	if projectConfig, ok := c.Projects[projectName]; ok {
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
	return c.DefaultOutputDir
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
