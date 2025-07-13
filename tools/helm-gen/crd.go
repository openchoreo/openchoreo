// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// copyCRDs copies CRD manifests from config/crd/bases to helm chart crds directory
func (g *Generator) copyCRDs() error {
	log.Println("Copying CRDs...")

	srcDir := filepath.Join(g.configDir, "crd", "bases")
	dstDir := filepath.Join(g.chartDir, "crds")

	// Ensure destination directory exists
	if err := ensureDir(dstDir); err != nil {
		return fmt.Errorf("failed to create CRD directory: %w", err)
	}

	// Read all CRD files from source directory
	crdFiles, err := filepath.Glob(filepath.Join(srcDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("failed to list CRD files: %w", err)
	}

	log.Printf("Found %d CRD files to copy", len(crdFiles))

	// Copy each CRD file
	for _, srcFile := range crdFiles {
		// Get just the filename
		filename := filepath.Base(srcFile)
		dstFile := filepath.Join(dstDir, filename)

		if err := g.copyCRDFile(srcFile, dstFile); err != nil {
			return fmt.Errorf("failed to copy CRD %s: %w", filename, err)
		}

		log.Printf("  Copied: %s -> %s", srcFile, dstFile)
	}

	return nil
}

// copyCRDFile copies a CRD file and removes the leading --- if present
func (g *Generator) copyCRDFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// Remove leading --- if present
	content = bytes.TrimPrefix(content, []byte("---\n"))
	
	return os.WriteFile(dst, content, 0644)
}
