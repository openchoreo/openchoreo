// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package names

import (
	"fmt"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

const (
	MaxWorkflowNameLength = 63
)

// MakeWorkflowName generates a valid workflow name with length constraints
func MakeWorkflowName(build *openchoreov1alpha1.Build) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(MaxWorkflowNameLength, build.Name)
}

// MakeNamespaceName generates the namespace name for the workflow based on organization
func MakeNamespaceName(build *openchoreov1alpha1.Build) string {
	orgName := normalizeForK8s(build.Namespace)
	return fmt.Sprintf("openchoreo-ci-%s", orgName)
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
