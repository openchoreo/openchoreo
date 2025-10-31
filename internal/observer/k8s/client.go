// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8s

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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Client wraps controller-runtime client
type Client struct {
	client client.Client
}

// NewClient creates a new Kubernetes client
func NewClient() (*Client, error) {
	config, err := ctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add batchv1 to scheme: %w", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Client{
		client: k8sClient,
	}, nil
}

// JobSpec defines the specification for creating an RCA job
type JobSpec struct {
	Name                    string
	Namespace               string
	ProjectID               string
	ComponentID             string
	Environment             string
	Timestamp               string
	ContextJSON             json.RawMessage
	ImageRepository         string
	ImageTag                string
	ImagePullPolicy         string
	TTLSecondsAfterFinished *int32
	ResourceLimitsCPU       string
	ResourceLimitsMemory    string
	ResourceRequestsCPU     string
	ResourceRequestsMemory  string
}

// CreateJob creates a Kubernetes job for RCA analysis
func (c *Client) CreateJob(ctx context.Context, spec JobSpec) (*batchv1.Job, error) {
	configMapName := fmt.Sprintf("%s-context", spec.Name)
	if len(spec.ContextJSON) > 0 && string(spec.ContextJSON) != "null" {
		if err := c.createContextConfigMap(ctx, spec.Namespace, configMapName, spec.ContextJSON); err != nil {
			return nil, fmt.Errorf("failed to create context configmap: %w", err)
		}
	}

	// TODO: Update proper labels in resources
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: spec.Namespace,
			Labels: map[string]string{
				"app":          "observability-rca-agent",
				"project-id":   spec.ProjectID,
				"component-id": spec.ComponentID,
				"environment":  spec.Environment,
				"managed-by":   "openchoreo-observer",
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: spec.TTLSecondsAfterFinished,
			BackoffLimit:            ptr.To[int32](3),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":          "rca-agent",
						"project-id":   spec.ProjectID,
						"component-id": spec.ComponentID,
						"environment":  spec.Environment,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            "rca-agent",
							Image:           fmt.Sprintf("%s:%s", spec.ImageRepository, spec.ImageTag),
							ImagePullPolicy: corev1.PullPolicy(spec.ImagePullPolicy),
							Env: []corev1.EnvVar{
								{Name: "PROJECT_ID", Value: spec.ProjectID},
								{Name: "COMPONENT_ID", Value: spec.ComponentID},
								{Name: "ENVIRONMENT", Value: spec.Environment},
								{Name: "TIMESTAMP", Value: spec.Timestamp},
							},
							Resources: buildResourceRequirements(spec),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "context",
									MountPath: "/etc/rca/context",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "context",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := c.client.Create(ctx, job); err != nil {
		if apierrors.IsForbidden(err) && strings.Contains(err.Error(), "exceeded quota") {
			return nil, fmt.Errorf("exceeded quota: %w", err)
		}
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

// GetJob retrieves a job by name and namespace
func (c *Client) GetJob(ctx context.Context, namespace, name string) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	if err := c.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, job); err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	return job, nil
}

// createContextConfigMap creates a ConfigMap containing the context JSON
func (c *Client) createContextConfigMap(ctx context.Context, namespace, name string, contextJSON json.RawMessage) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app":        "rca-agent",
				"managed-by": "openchoreo-observer",
			},
		},
		Data: map[string]string{
			"context.json": string(contextJSON),
		},
	}

	if err := c.client.Create(ctx, configMap); err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}

	return nil
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
