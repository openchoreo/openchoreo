// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode/config"
)

// Writer handles writing resources to files or stdout
type Writer struct {
	baseDir string
}

// NewWriter creates a new writer with the given base directory
func NewWriter(baseDir string) *Writer {
	return &Writer{baseDir: baseDir}
}

// WriteOptions configures the write operation
type WriteOptions struct {
	OutputDir       string // Optional: specific output directory
	DryRun          bool   // If true, write to stdout instead of file
	SkipIfUnchanged bool   // If true, skip writing if release is identical to latest
	Stdout          io.Writer
}

// WriteRelease writes a ComponentRelease to a file or stdout
// Returns the output path and a boolean indicating if the release was skipped (unchanged)
func (w *Writer) WriteRelease(release *unstructured.Unstructured, opts WriteOptions) (string, bool, error) {
	// If dry-run, write to stdout
	if opts.DryRun {
		data, err := yaml.Marshal(release.Object)
		if err != nil {
			return "", false, fmt.Errorf("failed to marshal release to YAML: %w", err)
		}
		writer := opts.Stdout
		if writer == nil {
			writer = os.Stdout
		}
		fmt.Fprintf(writer, "---\n")
		if _, err := writer.Write(data); err != nil {
			return "", false, fmt.Errorf("failed to write to stdout: %w", err)
		}
		return "", false, nil
	}

	// Determine initial output path (may need version increment)
	initialPath := w.determineOutputPath(release, opts)
	outputDir := filepath.Dir(initialPath)
	componentName := getNestedString(release.Object, "spec", "owner", "componentName")

	// Check for duplicate if requested
	if opts.SkipIfUnchanged {
		latestRelease, _, err := FindLatestRelease(outputDir, componentName)
		if err != nil {
			// Log warning but continue (don't fail on duplicate check errors)
			fmt.Fprintf(os.Stderr, "Warning: failed to check for existing release: %v\n", err)
		} else if latestRelease != nil {
			identical, err := CompareReleaseSpecs(release, latestRelease)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to compare releases: %v\n", err)
			} else if identical {
				// Release is identical to latest - skip writing
				return initialPath, true, nil
			}
		}
	}

	// Auto-increment version if the file would overwrite an existing one
	finalRelease := release.DeepCopy()
	outputPath := initialPath

	if _, err := os.Stat(outputPath); err == nil {
		// File exists - need to increment version
		releaseName := finalRelease.GetName()

		// Extract date from release name (format: component-YYYYMMDD-version)
		parts := strings.Split(releaseName, "-")
		if len(parts) >= 3 {
			dateStr := parts[len(parts)-2]

			// Get next version number
			nextVersion, err := GetNextVersionNumber(outputDir, componentName, dateStr)
			if err != nil {
				return "", false, fmt.Errorf("failed to determine next version: %w", err)
			}

			// Update release name with new version
			parts[len(parts)-1] = nextVersion
			newReleaseName := strings.Join(parts, "-")
			finalRelease.SetName(newReleaseName)
			outputPath = filepath.Join(outputDir, newReleaseName+".yaml")
		}
	}

	// Marshal to YAML
	data, err := yaml.Marshal(finalRelease.Object)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal release to YAML: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return "", false, fmt.Errorf("failed to write release file: %w", err)
	}

	return outputPath, false, nil
}

// determineOutputPath determines the output file path based on options and release metadata
func (w *Writer) determineOutputPath(release *unstructured.Unstructured, opts WriteOptions) string {
	releaseName := release.GetName()

	if opts.OutputDir != "" {
		return filepath.Join(opts.OutputDir, releaseName+".yaml")
	}

	// Default: projects/<project>/components/<component>/releases/
	projectName := getNestedString(release.Object, "spec", "owner", "projectName")
	componentName := getNestedString(release.Object, "spec", "owner", "componentName")

	return filepath.Join(
		w.baseDir,
		"projects", projectName,
		"components", componentName,
		"releases",
		releaseName+".yaml",
	)
}

// getNestedString safely extracts a nested string value
func getNestedString(obj map[string]interface{}, fields ...string) string {
	current := obj
	for i, field := range fields {
		if i == len(fields)-1 {
			if val, ok := current[field].(string); ok {
				return val
			}
			return ""
		}
		if next, ok := current[field].(map[string]interface{}); ok {
			current = next
		} else {
			return ""
		}
	}
	return ""
}

// WriteResource writes any unstructured resource to a file or stdout
func (w *Writer) WriteResource(resource *unstructured.Unstructured, outputPath string, dryRun bool) error {
	data, err := yaml.Marshal(resource.Object)
	if err != nil {
		return fmt.Errorf("failed to marshal resource to YAML: %w", err)
	}

	if dryRun {
		fmt.Println("---")
		fmt.Print(string(data))
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// OutputDirResolverFunc resolves the output directory for a given project and component.
// Returns the directory path, or empty string to fall through to the default.
type OutputDirResolverFunc func(projectName, componentName string) string

// BulkWriteOptions configures bulk write operations
type BulkWriteOptions struct {
	Config          *config.ReleaseConfig // Config for output directory resolution
	OutputDir       string                // Default output directory
	Resolver        OutputDirResolverFunc // Optional resolver for output directory
	DryRun          bool                  // If true, write to stdout
	SkipIfUnchanged bool                  // If true, skip writing if release is identical to latest
	Stdout          io.Writer             // Writer for dry-run output
}

// BulkWriteResult contains the result of a bulk write operation
type BulkWriteResult struct {
	OutputPaths []string // Paths where files were written (empty for dry-run)
	Skipped     []string // Release names that were skipped (unchanged)
	Errors      []error  // Any errors encountered during writing
}

// WriteBulkReleases writes multiple releases according to config
func (w *Writer) WriteBulkReleases(
	releases []*unstructured.Unstructured,
	opts BulkWriteOptions,
) (*BulkWriteResult, error) {
	result := &BulkWriteResult{
		OutputPaths: make([]string, 0, len(releases)),
		Skipped:     make([]string, 0),
		Errors:      make([]error, 0),
	}

	// If dry-run, concatenate all releases to stdout
	if opts.DryRun {
		writer := opts.Stdout
		if writer == nil {
			writer = os.Stdout
		}

		for i, release := range releases {
			data, err := yaml.Marshal(release.Object)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to marshal release %s: %w", release.GetName(), err))
				continue
			}

			if i > 0 {
				fmt.Fprintf(writer, "\n")
			}
			fmt.Fprintf(writer, "---\n")
			if _, err := writer.Write(data); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to write release %s to stdout: %w", release.GetName(), err))
				continue
			}
		}
		return result, nil
	}

	// Write each release to its resolved output directory
	for _, release := range releases {
		projectName := getNestedString(release.Object, "spec", "owner", "projectName")
		componentName := getNestedString(release.Object, "spec", "owner", "componentName")

		outputPath := w.resolveOutputPath(release, projectName, componentName, opts)

		// Check for duplicate if requested
		if opts.SkipIfUnchanged {
			outputDir := filepath.Dir(outputPath)
			latestRelease, _, err := FindLatestRelease(outputDir, componentName)
			if err != nil {
				// Log warning but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to check for existing release %s: %v\n", componentName, err)
			} else if latestRelease != nil {
				identical, err := CompareReleaseSpecs(release, latestRelease)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to compare releases for %s: %v\n", componentName, err)
				} else if identical {
					// Release is identical to latest - skip writing
					result.Skipped = append(result.Skipped, release.GetName())
					continue
				}
			}
		}

		// Marshal to YAML
		data, err := yaml.Marshal(release.Object)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to marshal release %s: %w", release.GetName(), err))
			continue
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create directory for %s: %w", release.GetName(), err))
			continue
		}

		// Write file
		if err := os.WriteFile(outputPath, data, 0600); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write release %s: %w", release.GetName(), err))
			continue
		}

		result.OutputPaths = append(result.OutputPaths, outputPath)
	}

	return result, nil
}

// resolveOutputPath determines the output file path based on config and options
func (w *Writer) resolveOutputPath(
	release *unstructured.Unstructured,
	projectName, componentName string,
	opts BulkWriteOptions,
) string {
	releaseName := release.GetName()

	// Priority 1: Check config file for component-specific or project-specific path
	if opts.Config != nil {
		if configDir := opts.Config.GetReleaseOutputDir(projectName, componentName); configDir != "" {
			// If config path is relative, resolve it against baseDir
			if !filepath.IsAbs(configDir) {
				configDir = filepath.Join(w.baseDir, configDir)
			}
			return filepath.Join(configDir, releaseName+".yaml")
		}
	}

	// Priority 2: Use --output flag if provided
	if opts.OutputDir != "" {
		// If output dir is relative, resolve it against baseDir
		outputDir := opts.OutputDir
		if !filepath.IsAbs(outputDir) {
			outputDir = filepath.Join(w.baseDir, outputDir)
		}
		return filepath.Join(outputDir, releaseName+".yaml")
	}

	// Priority 3: Use resolver function if provided
	if opts.Resolver != nil {
		if resolvedDir := opts.Resolver(projectName, componentName); resolvedDir != "" {
			if !filepath.IsAbs(resolvedDir) {
				resolvedDir = filepath.Join(w.baseDir, resolvedDir)
			}
			return filepath.Join(resolvedDir, releaseName+".yaml")
		}
	}

	// Priority 4: Use default path structure
	return filepath.Join(
		w.baseDir,
		"projects", projectName,
		"components", componentName,
		"releases",
		releaseName+".yaml",
	)
}

// BulkBindingWriteOptions configures bulk binding write operations
type BulkBindingWriteOptions struct {
	Config    *config.ReleaseConfig // Config for output directory resolution
	OutputDir string                // Default output directory
	Resolver  OutputDirResolverFunc // Optional resolver for output directory
	DryRun    bool                  // If true, write to stdout
	Stdout    io.Writer             // Writer for dry-run output
}

// WriteBinding writes a ReleaseBinding to a file or stdout
// Returns the output path and a boolean (always false for bindings, kept for consistency)
func (w *Writer) WriteBinding(binding *unstructured.Unstructured, opts WriteOptions) (string, bool, error) {
	// If dry-run, write to stdout
	if opts.DryRun {
		data, err := yaml.Marshal(binding.Object)
		if err != nil {
			return "", false, fmt.Errorf("failed to marshal binding to YAML: %w", err)
		}
		writer := opts.Stdout
		if writer == nil {
			writer = os.Stdout
		}
		fmt.Fprintf(writer, "---\n")
		if _, err := writer.Write(data); err != nil {
			return "", false, fmt.Errorf("failed to write to stdout: %w", err)
		}
		return "", false, nil
	}

	// Determine output path
	outputPath := w.determineBindingOutputPath(binding, opts.OutputDir)

	// Marshal to YAML
	data, err := yaml.Marshal(binding.Object)
	if err != nil {
		return "", false, fmt.Errorf("failed to marshal binding to YAML: %w", err)
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return "", false, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(outputPath, data, 0600); err != nil {
		return "", false, fmt.Errorf("failed to write binding file: %w", err)
	}

	return outputPath, false, nil
}

// determineBindingOutputPath determines the output file path for a binding
func (w *Writer) determineBindingOutputPath(binding *unstructured.Unstructured, outputDir string) string {
	bindingName := binding.GetName()

	if outputDir != "" {
		return filepath.Join(outputDir, bindingName+".yaml")
	}

	// Default: projects/<project>/components/<component>/bindings/
	projectName := getNestedString(binding.Object, "spec", "owner", "projectName")
	componentName := getNestedString(binding.Object, "spec", "owner", "componentName")

	return filepath.Join(
		w.baseDir,
		"projects", projectName,
		"components", componentName,
		"bindings",
		bindingName+".yaml",
	)
}

// WriteBulkBindings writes multiple bindings according to config
func (w *Writer) WriteBulkBindings(
	bindings []*unstructured.Unstructured,
	opts BulkBindingWriteOptions,
) (*BulkWriteResult, error) {
	result := &BulkWriteResult{
		OutputPaths: make([]string, 0, len(bindings)),
		Skipped:     make([]string, 0),
		Errors:      make([]error, 0),
	}

	// If dry-run, concatenate all bindings to stdout
	if opts.DryRun {
		writer := opts.Stdout
		if writer == nil {
			writer = os.Stdout
		}

		for i, binding := range bindings {
			data, err := yaml.Marshal(binding.Object)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to marshal binding %s: %w", binding.GetName(), err))
				continue
			}

			if i > 0 {
				fmt.Fprintf(writer, "\n")
			}
			fmt.Fprintf(writer, "---\n")
			if _, err := writer.Write(data); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to write binding %s to stdout: %w", binding.GetName(), err))
				continue
			}
		}
		return result, nil
	}

	// Write each binding to its resolved output directory
	for _, binding := range bindings {
		projectName := getNestedString(binding.Object, "spec", "owner", "projectName")
		componentName := getNestedString(binding.Object, "spec", "owner", "componentName")

		outputPath := w.resolveBindingOutputPath(binding, projectName, componentName, opts)

		// Marshal to YAML
		data, err := yaml.Marshal(binding.Object)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to marshal binding %s: %w", binding.GetName(), err))
			continue
		}

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create directory for %s: %w", binding.GetName(), err))
			continue
		}

		// Write file
		if err := os.WriteFile(outputPath, data, 0600); err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to write binding %s: %w", binding.GetName(), err))
			continue
		}

		result.OutputPaths = append(result.OutputPaths, outputPath)
	}

	return result, nil
}

// resolveBindingOutputPath determines the output file path for a binding based on config and options
func (w *Writer) resolveBindingOutputPath(
	binding *unstructured.Unstructured,
	projectName, componentName string,
	opts BulkBindingWriteOptions,
) string {
	bindingName := binding.GetName()

	// Priority 1: Check config file for component-specific or project-specific path
	if opts.Config != nil {
		if configDir := opts.Config.GetBindingOutputDir(projectName, componentName); configDir != "" {
			// If config path is relative, resolve it against baseDir
			if !filepath.IsAbs(configDir) {
				configDir = filepath.Join(w.baseDir, configDir)
			}
			return filepath.Join(configDir, bindingName+".yaml")
		}
	}

	// Priority 2: Use --output flag if provided
	if opts.OutputDir != "" {
		// If output dir is relative, resolve it against baseDir
		outputDir := opts.OutputDir
		if !filepath.IsAbs(outputDir) {
			outputDir = filepath.Join(w.baseDir, outputDir)
		}
		return filepath.Join(outputDir, bindingName+".yaml")
	}

	// Priority 3: Use resolver function if provided
	if opts.Resolver != nil {
		if resolvedDir := opts.Resolver(projectName, componentName); resolvedDir != "" {
			if !filepath.IsAbs(resolvedDir) {
				resolvedDir = filepath.Join(w.baseDir, resolvedDir)
			}
			return filepath.Join(resolvedDir, bindingName+".yaml")
		}
	}

	// Priority 4: Use default path structure
	// projects/<project>/components/<component>/bindings/<binding>.yaml
	return filepath.Join(
		w.baseDir,
		"projects", projectName,
		"components", componentName,
		"bindings",
		bindingName+".yaml",
	)
}
