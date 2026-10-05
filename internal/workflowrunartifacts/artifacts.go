// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

// Package workflowrunartifacts validates and projects immutable WorkflowRun inputs.
package workflowrunartifacts

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const maxTTL = 7 * 24 * time.Hour

var keyPattern = regexp.MustCompile(`^unified-diff/[A-Za-z0-9][A-Za-z0-9._-]{0,127}/([a-f0-9]{64})\.diff$`)
var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$`)

// Validate rejects unsafe, mutable, expired, or oversized artifact references.
// It is used both at the API boundary and immediately before runner submission,
// so direct Kubernetes clients receive the same safety guarantee.
func Validate(artifacts []v1alpha1.WorkflowRunInputArtifact, now time.Time) error {
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		if _, ok := seen[artifact.Name]; ok {
			return fmt.Errorf("input artifact %q is duplicated", artifact.Name)
		}
		seen[artifact.Name] = struct{}{}
		if artifact.SizeBytes < 1 || artifact.SizeBytes > v1alpha1.WorkflowRunInputArtifactMaxSizeBytes {
			return fmt.Errorf("input artifact %q sizeBytes must be between 1 and %d", artifact.Name, v1alpha1.WorkflowRunInputArtifactMaxSizeBytes)
		}
		if artifact.ExpiresAt.Time.Before(now) || artifact.ExpiresAt.Time.Equal(now) {
			return fmt.Errorf("input artifact %q is expired", artifact.Name)
		}
		if artifact.ExpiresAt.Time.After(now.Add(maxTTL)) {
			return fmt.Errorf("input artifact %q expires more than %s from now", artifact.Name, maxTTL)
		}
		if err := validateGCS(artifact); err != nil {
			return err
		}
	}
	return nil
}

func validateGCS(artifact v1alpha1.WorkflowRunInputArtifact) error {
	if artifact.Name != v1alpha1.WorkflowRunInputArtifactName {
		return fmt.Errorf("input artifact name must be %q", v1alpha1.WorkflowRunInputArtifactName)
	}
	if artifact.MediaType != "text/x-diff" {
		return fmt.Errorf("input artifact %q mediaType must be text/x-diff", artifact.Name)
	}
	if !bucketPattern.MatchString(artifact.GCS.Bucket) || artifact.GCS.Bucket != strings.ToLower(artifact.GCS.Bucket) {
		return fmt.Errorf("input artifact %q must provide a valid lowercase GCS bucket", artifact.Name)
	}
	matches := keyPattern.FindStringSubmatch(artifact.GCS.Key)
	if matches == nil || matches[1] != artifact.SHA256 {
		return fmt.Errorf("input artifact %q GCS key must be unified-diff/<delivery-id>/<sha256>.diff", artifact.Name)
	}
	return nil
}

// StatusMetadata returns exactly the metadata that is safe to expose through
// WorkflowRun status. References and payload contents remain absent.
func StatusMetadata(artifacts []v1alpha1.WorkflowRunInputArtifact) []v1alpha1.WorkflowRunInputArtifactStatus {
	if len(artifacts) == 0 {
		return nil
	}
	result := make([]v1alpha1.WorkflowRunInputArtifactStatus, len(artifacts))
	for i, artifact := range artifacts {
		result[i] = v1alpha1.WorkflowRunInputArtifactStatus{
			Name: artifact.Name, MediaType: artifact.MediaType, SizeBytes: artifact.SizeBytes,
			SHA256: artifact.SHA256, ExpiresAt: metav1.NewTime(artifact.ExpiresAt.Time),
		}
	}
	return result
}
