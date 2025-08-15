// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

const (
	MaxWorkflowNameLength = 63
	MaxImageNameLength    = 63
	MaxImageTagLength     = 128
	DefaultDTName         = "default"
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

// MakeWorkflowName generates a valid workflow name with length constraints
func MakeWorkflowName(build *openchoreov1alpha1.Build) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(MaxWorkflowNameLength, build.Name)
}

// MakeNamespaceName generates the namespace name for the workflow based on organization
func MakeNamespaceName(build *openchoreov1alpha1.Build) string {
	orgName := normalizeForK8s(build.Namespace)
	return fmt.Sprintf("openchoreo-ci-%s", orgName)
}

// MakeWorkflowLabels creates labels for the workflow
func MakeWorkflowLabels(build *openchoreov1alpha1.Build) map[string]string {
	labels := map[string]string{
		dpkubernetes.LabelKeyOrganizationName: build.Namespace,
		dpkubernetes.LabelKeyProjectName:      build.Spec.Owner.ProjectName,
		dpkubernetes.LabelKeyComponentName:    build.Spec.Owner.ComponentName,
		dpkubernetes.LabelKeyBuildName:        build.Name,
		dpkubernetes.LabelKeyUUID:             string(build.UID),
		dpkubernetes.LabelKeyTarget:           dpkubernetes.LabelValueBuildTarget,
	}
	return labels
}

// MakeNamespace creates a namespace for the build
func MakeNamespace(build *openchoreov1alpha1.Build) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   MakeNamespaceName(build),
			Labels: MakeWorkflowLabels(build),
		},
	}
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

// normalizeForK8s normalizes a string to be valid for Kubernetes labels/names
func normalizeForK8s(s string) string {
	// Replace invalid characters with hyphens
	normalized := strings.ReplaceAll(s, "_", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")
	normalized = strings.ToLower(normalized)

	// Ensure it starts and ends with alphanumeric characters
	normalized = strings.Trim(normalized, "-")

	// Limit length for labels (63 characters max)
	if len(normalized) > 63 {
		normalized = normalized[:63]
		normalized = strings.TrimSuffix(normalized, "-")
	}

	return normalized
}
