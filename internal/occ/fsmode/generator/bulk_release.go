// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package generator

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openchoreo/openchoreo/internal/occ/fsmode/typed"
	"github.com/openchoreo/openchoreo/pkg/fsindex/index"
)

// BulkReleaseOptions configures bulk release generation
type BulkReleaseOptions struct {
	ProjectName string // If set, only release components in this project
	All         bool   // If true, release all components
	Version     string // Version string for all releases
	Namespace   string // Kubernetes namespace
}

// BulkReleaseResult represents the result of a bulk release operation
type BulkReleaseResult struct {
	Releases []ReleaseInfo
	Errors   []ReleaseError
}

// ReleaseInfo contains info about a generated release
type ReleaseInfo struct {
	ComponentName string
	ProjectName   string
	ReleaseName   string
	Release       *unstructured.Unstructured
}

// ReleaseError contains info about a failed release
type ReleaseError struct {
	ComponentName string
	ProjectName   string
	Error         error
}

// GenerateBulkReleases generates releases for multiple components
func (g *ReleaseGenerator) GenerateBulkReleases(opts BulkReleaseOptions) (*BulkReleaseResult, error) {
	// Discover components to process
	components, err := g.discoverComponents(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to discover components: %w", err)
	}

	if len(components) == 0 {
		return nil, fmt.Errorf("no components found to release")
	}

	// Generate releases for each component
	result := &BulkReleaseResult{
		Releases: make([]ReleaseInfo, 0, len(components)),
		Errors:   make([]ReleaseError, 0),
	}

	for _, compEntry := range components {
		comp, err := typed.NewComponent(compEntry)
		if err != nil {
			result.Errors = append(result.Errors, ReleaseError{
				ComponentName: compEntry.Name(),
				ProjectName:   compEntry.GetNestedString("spec", "owner", "projectName"),
				Error:         fmt.Errorf("failed to convert component: %w", err),
			})
			continue
		}

		releaseOpts := ReleaseOptions{
			ComponentName: comp.Name,
			ProjectName:   comp.ProjectName(),
			Version:       opts.Version,
			Namespace:     opts.Namespace,
		}

		release, err := g.GenerateRelease(releaseOpts)
		if err != nil {
			result.Errors = append(result.Errors, ReleaseError{
				ComponentName: comp.Name,
				ProjectName:   comp.ProjectName(),
				Error:         err,
			})
			continue
		}

		result.Releases = append(result.Releases, ReleaseInfo{
			ComponentName: comp.Name,
			ProjectName:   comp.ProjectName(),
			ReleaseName:   release.GetName(),
			Release:       release,
		})
	}

	return result, nil
}

// discoverComponents discovers which components to process based on options
func (g *ReleaseGenerator) discoverComponents(opts BulkReleaseOptions) ([]*index.ResourceEntry, error) {
	if opts.All {
		// Return all components in the repository
		return g.index.ListComponents(), nil
	}

	if opts.ProjectName != "" {
		// Return all components in the specified project
		components := g.index.ListComponentsForProject(opts.ProjectName)
		if len(components) == 0 {
			return nil, fmt.Errorf("no components found for project %q", opts.ProjectName)
		}
		return components, nil
	}

	return nil, fmt.Errorf("either --all or --project must be specified for bulk release")
}
