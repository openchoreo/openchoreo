// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package names

import (
	"fmt"
	"regexp"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	MaxImageNameLength = 63
	MaxImageTagLength  = 128
	DefaultDTName      = "default"
)

// MakeImageName creates the image name following the pattern: project_name-component_name
func MakeImageName(build *openchoreov1alpha1.Build) string {
	projectName := normalizeForImageName(build.Spec.Owner.ProjectName)
	componentName := normalizeForImageName(build.Spec.Owner.ComponentName)

	imageName := fmt.Sprintf("%s-%s", projectName, componentName)

	// Ensure image name doesn't exceed maximum length
	if len(imageName) > MaxImageNameLength {
		imageName = imageName[:MaxImageNameLength]
		// Remove any trailing hyphens
		imageName = strings.TrimSuffix(imageName, "-")
	}

	return imageName
}

// MakeImageTag creates the image tag
func MakeImageTag(build *openchoreov1alpha1.Build) string {
	return DefaultDTName
}

// normalizeForImageName normalizes a string for use in image names
// Docker image names must be lowercase and can contain only alphanumeric characters, hyphens, and underscores
func normalizeForImageName(s string) string {
	// Convert to lowercase
	normalized := strings.ToLower(s)

	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9\-_]`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Remove leading and trailing hyphens
	normalized = strings.Trim(normalized, "-")

	return normalized
}
