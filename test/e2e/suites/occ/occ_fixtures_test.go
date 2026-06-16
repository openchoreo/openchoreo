// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"
	clusterDataPlane   = "e2e-shared"
	projectName        = "occ-proj"
	envDev             = "development"
	envStaging         = "staging"
	componentName      = "occ-echo"
)

var occRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-occ-%s", occRunID)

// Cluster-scoped names include the run ID to avoid collisions between parallel runs.
var (
	clusterAuthzRoleName    = fmt.Sprintf("occ-e2e-role-%s", occRunID)
	clusterAuthzBindingName = fmt.Sprintf("occ-e2e-binding-%s", occRunID)
)

const (
	authzRoleName        = "occ-e2e-reader"
	authzRoleBindingName = "occ-e2e-reader-binding"
	secretRefName        = "occ-e2e-secret"
	oancName             = "occ-e2e-webhook"
)

func mustYAMLDocs(objects ...any) string {
	docs := make([]string, 0, len(objects))
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal yaml document: %v", err))
		}
		docs = append(docs, strings.TrimSpace(string(data)))
	}
	return strings.Join(docs, "\n---\n")
}

func cpNamespaceYAML() string {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
	return mustYAMLDocs(ns)
}

func platformResourcesYAML() string {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": "default"},
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{
					{Name: envStaging},
				},
			}},
		},
	}
	envs := make([]any, 0, 2)
	for _, name := range []string{envDev, envStaging} {
		envs = append(envs, &openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: cpNs,
				Labels:    map[string]string{"openchoreo.dev/name": name},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
			},
		})
	}
	proj := &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": projectName},
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type:                  openchoreov1alpha1.ProjectTypeRef{Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType, Name: "default"},
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj)
	return mustYAMLDocs(docs...)
}

func componentWithWorkloadYAML() string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": componentName},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/service",
			},
			AutoDeploy: true,
		},
	}
	workload := occWorkload(map[string]string{"openchoreo.dev/name": componentName})
	return mustYAMLDocs(comp, workload)
}

func workloadUpdatedYAML() string {
	return mustYAMLDocs(occWorkload(map[string]string{
		"openchoreo.dev/name": componentName,
		"e2e-updated":         "true",
	}))
}

func renderedReleaseYAML() string {
	rr := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "RenderedRelease",
		"metadata": map[string]any{
			"name":      "fake-rr",
			"namespace": cpNs,
		},
		"spec": map[string]any{},
	}
	data, err := yaml.Marshal(rr)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal yaml: %v", err))
	}
	return strings.TrimSpace(string(data))
}

func occWorkload(labels map[string]string) *openchoreov1alpha1.Workload {
	return &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: cpNs,
			Labels:    labels,
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {
						Type: openchoreov1alpha1.EndpointType("HTTP"),
						Port: 8080,
						Visibility: []openchoreov1alpha1.EndpointVisibility{
							openchoreov1alpha1.EndpointVisibilityProject,
						},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: "hashicorp/http-echo:0.2.3",
					Args:  []string{"-listen=:8080", "-text=occ-e2e"},
				},
			},
		},
	}
}

func clusterAuthzRoleYAML() string {
	role := &openchoreov1alpha1.ClusterAuthzRole{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ClusterAuthzRole"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterAuthzRoleName,
			Labels: map[string]string{"openchoreo.dev/name": clusterAuthzRoleName},
		},
		Spec: openchoreov1alpha1.ClusterAuthzRoleSpec{
			Actions: []string{"read"},
		},
	}
	return mustYAMLDocs(role)
}

func authzRoleWithBindingYAML() string {
	role := &openchoreov1alpha1.AuthzRole{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "AuthzRole"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      authzRoleName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": authzRoleName},
		},
		Spec: openchoreov1alpha1.AuthzRoleSpec{
			Actions: []string{"read", "write"},
		},
	}
	binding := &openchoreov1alpha1.AuthzRoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "AuthzRoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      authzRoleBindingName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": authzRoleBindingName},
		},
		Spec: openchoreov1alpha1.AuthzRoleBindingSpec{
			Entitlement: openchoreov1alpha1.EntitlementClaim{
				Claim: "sub",
				Value: clientID,
			},
			RoleMappings: []openchoreov1alpha1.RoleMapping{{
				RoleRef: openchoreov1alpha1.RoleRef{
					Name: authzRoleName,
					Kind: openchoreov1alpha1.RoleRefKindAuthzRole,
				},
			}},
			Effect: openchoreov1alpha1.EffectAllow,
		},
	}
	return mustYAMLDocs(role, binding)
}

func secretReferenceYAML() string {
	ref := &openchoreov1alpha1.SecretReference{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "SecretReference"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRefName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": secretRefName},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{
				Kind: "ClusterDataPlane",
				Name: "default",
			},
			Template: openchoreov1alpha1.SecretTemplate{
				Type: corev1.SecretTypeOpaque,
			},
			Data: []openchoreov1alpha1.SecretDataSource{{
				SecretKey: "token",
				RemoteRef: openchoreov1alpha1.RemoteReference{
					Key:      "dev/dummy-token",
					Property: "value",
				},
			}},
		},
	}
	return mustYAMLDocs(ref)
}

func oancWebhookYAML() string {
	channel := &openchoreov1alpha1.ObservabilityAlertsNotificationChannel{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ObservabilityAlertsNotificationChannel"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      oancName,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": oancName},
		},
		Spec: openchoreov1alpha1.ObservabilityAlertsNotificationChannelSpec{
			Type:        openchoreov1alpha1.NotificationChannelTypeWebhook,
			Environment: envDev,
			WebhookConfig: &openchoreov1alpha1.WebhookConfig{
				URL: "http://localhost:9999/webhook",
			},
		},
	}
	return mustYAMLDocs(channel)
}
