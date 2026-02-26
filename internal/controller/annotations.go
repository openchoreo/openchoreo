// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import "gopkg.in/yaml.v3"

// This file contains the all the annotations that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	AnnotationKeyDisplayName = "openchoreo.dev/display-name"
	AnnotationKeyDescription = "openchoreo.dev/description"

	// AnnotationKeyComponentWorkflowParameters maps logical parameter keys (repoUrl, branch, appPath, secretRef, commit)
	// to dotted parameter paths within the workflow schema. Used to identify which parameters hold
	// component build information for webhook auto-build and workflow triggering.
	// Format: YAML multi-line block scalar with "key: value" pairs per line.
	// Example:
	//   backstage.io/component-workflow-parameters: |
	//     repoUrl: parameters.repository.url
	//     branch: parameters.repository.revision.branch
	AnnotationKeyComponentWorkflowParameters = "backstage.io/component-workflow-parameters"
)

// ParseWorkflowParameterAnnotation parses the component-workflow-parameters annotation
// from YAML format (key: value pairs) into a map.
func ParseWorkflowParameterAnnotation(annotation string) map[string]string {
	result := make(map[string]string)
	if annotation == "" {
		return result
	}
	_ = yaml.Unmarshal([]byte(annotation), &result)
	return result
}
