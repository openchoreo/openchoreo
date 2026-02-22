// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

// This file contains the all the annotations that are used to store Choreo specific the metadata in the Kubernetes objects.

const (
	AnnotationKeyDisplayName = "openchoreo.dev/display-name"
	AnnotationKeyDescription = "openchoreo.dev/description"

	// AnnotationKeyComponentWorkflowParameters maps logical parameter keys (repoUrl, branch, appPath, secretRef, commit)
	// to dotted parameter paths within the workflow schema. Used to identify which parameters hold
	// component build information for webhook auto-build and workflow triggering.
	// Format: "key1: path1, key2: path2, ..."
	// Example: "repoUrl: parameters.repository.url, branch: parameters.repository.revision.branch, appPath: parameters.repository.appPath, commit: parameters.repository.revision.commit"
	AnnotationKeyComponentWorkflowParameters = "openchoreo.dev/component-workflow-parameters"
)
