// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Generator handles the generation of Helm chart resources from Kubernetes manifests
type Generator struct {
	configDir        string // Path to config/ directory
	chartDir         string // Path to helm chart directory
	controllerSubDir string // Subdirectory for controller resources (e.g., "controller" or "generated/controller")
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

// Helper function to ensure a directory exists
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0755)
}

// Helper function to copy a file
func copyFile(src, dst string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, input, 0644)
}

// controllerDir returns the full path to the controller subdirectory within the helm templates
func (g *Generator) controllerDir() string {
	return filepath.Join(g.chartDir, "templates", g.controllerSubDir)
}
