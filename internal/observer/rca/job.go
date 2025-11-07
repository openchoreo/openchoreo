// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package rca

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// JobSpec defines the specification for creating an RCA job
type JobSpec struct {
	Name                    string
	Namespace               string
	ImageRepository         string
	ImageTag                string
	ImagePullPolicy         string
	TTLSecondsAfterFinished *int32
	ResourceLimitsCPU       string
	ResourceLimitsMemory    string
	ResourceRequestsCPU     string
	ResourceRequestsMemory  string
}

// RCAContext defines the runtime context for an RCA job
type RCAContext struct {
	RCAID    string          `json:"rca_id"`
	Metadata json.RawMessage `json:"metadata"`
}

// CreateJob creates a Kubernetes job for RCA analysis
func CreateJob(ctx context.Context, k8sClient client.Client, spec JobSpec, rcaCtx RCAContext) (*batchv1.Job, error) {
	// Marshal the entire RCA context to JSON
	contextJSON, err := json.Marshal(rcaCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal RCA context: %w", err)
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels:    map[string]string{},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: spec.TTLSecondsAfterFinished,
			BackoffLimit:            ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "observability-rca-agent",
							Image:           fmt.Sprintf("%s:%s", spec.ImageRepository, spec.ImageTag),
							ImagePullPolicy: corev1.PullPolicy(spec.ImagePullPolicy),
							Resources:       buildResourceRequirements(spec),
							Args: []string{
								"--context", string(contextJSON),
							},
						},
					},
				},
			},
		},
	}

	if err := k8sClient.Create(ctx, job); err != nil {
		if apierrors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota") {
			return nil, fmt.Errorf("exceeded quota: %w", err)
		}
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// buildResourceRequirements creates ResourceRequirements from JobSpec
func buildResourceRequirements(spec JobSpec) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Parse and set CPU requests
	if spec.ResourceRequestsCPU != "" {
		if qty, err := resource.ParseQuantity(spec.ResourceRequestsCPU); err == nil {
			requirements.Requests[corev1.ResourceCPU] = qty
		}
	}

	// Parse and set Memory requests
	if spec.ResourceRequestsMemory != "" {
		if qty, err := resource.ParseQuantity(spec.ResourceRequestsMemory); err == nil {
			requirements.Requests[corev1.ResourceMemory] = qty
		}
	}

	// Parse and set CPU limits
	if spec.ResourceLimitsCPU != "" {
		if qty, err := resource.ParseQuantity(spec.ResourceLimitsCPU); err == nil {
			requirements.Limits[corev1.ResourceCPU] = qty
		}
	}

	// Parse and set Memory limits
	if spec.ResourceLimitsMemory != "" {
		if qty, err := resource.ParseQuantity(spec.ResourceLimitsMemory); err == nil {
			requirements.Limits[corev1.ResourceMemory] = qty
		}
	}

	return requirements
}
