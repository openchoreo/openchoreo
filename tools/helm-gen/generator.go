// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Generator handles the generation of Helm chart resources from Kubernetes manifests
type Generator struct {
	configDir        string // Path to config/ directory
	chartDir         string // Path to helm chart directory
	controllerSubDir string // Subdirectory for controller resources (e.g., "controller" or "generated/controller")
	chartName        string // Chart name from Chart.yaml
}

// Chart represents the structure of Chart.yaml
type Chart struct {
	Name string `yaml:"name"`
}

// NewGenerator creates a new Generator instance
func NewGenerator(configDir, chartDir, controllerSubDir string) *Generator {
	return &Generator{
		configDir:        configDir,
		chartDir:         chartDir,
		controllerSubDir: controllerSubDir,
	}
}

// Run executes the helm chart generation process
func (g *Generator) Run() error {
	log.Printf("Generating Helm chart from config: %s to chart: %s", g.configDir, g.chartDir)

	// Step 0: Read chart name from Chart.yaml
	if err := g.readChartName(); err != nil {
		return fmt.Errorf("failed to read chart name: %w", err)
	}

	// Step 1: Copy CRDs
	if err := g.copyCRDs(); err != nil {
		return fmt.Errorf("failed to copy CRDs: %w", err)
	}

	// Step 2: Generate RBAC resources
	if err := g.generateRBAC(); err != nil {
		return fmt.Errorf("failed to generate RBAC: %w", err)
	}

	// Step 3: Generate webhook configurations
	if err := g.generateWebhooks(); err != nil {
		return fmt.Errorf("failed to generate webhooks: %w", err)
	}

	return nil
}

// readChartName reads the chart name from Chart.yaml
func (g *Generator) readChartName() error {
	chartFile := filepath.Join(g.chartDir, "Chart.yaml")
	content, err := os.ReadFile(chartFile)
	if err != nil {
		return fmt.Errorf("failed to read Chart.yaml: %w", err)
	}

	var chart Chart
	if err := yaml.Unmarshal(content, &chart); err != nil {
		return fmt.Errorf("failed to parse Chart.yaml: %w", err)
	}

	g.chartName = chart.Name
	log.Printf("Chart name: %s", g.chartName)
	return nil
}

// Helper function to ensure a directory exists
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// controllerDir returns the full path to the controller subdirectory within the helm templates
func (g *Generator) controllerDir() string {
	return filepath.Join(g.chartDir, "templates", g.controllerSubDir)
}
