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
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"
	projectName        = "proj1"
	environmentName    = "development"
	componentName      = "secret-app"
)

var secretsRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-secrets-%s", secretsRunID)

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
	docs := []any{
		&openchoreov1alpha1.DeploymentPipeline{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/name": "default",
				},
			},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{
				PromotionPaths: []openchoreov1alpha1.PromotionPath{
					{
						SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: environmentName},
						TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: "staging"}},
					},
				},
			},
		},
		&openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      environmentName,
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/name": environmentName,
				},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
				IsProduction: false,
			},
		},
		&openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "staging",
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/name": "staging",
				},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
				IsProduction: false,
			},
		},
		&openchoreov1alpha1.Project{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      projectName,
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/name": projectName,
				},
			},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
				Type:                  openchoreov1alpha1.ProjectTypeRef{Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType, Name: "default"},
			},
		},
	}
	return mustYAMLDocs(docs...)
}

func secretReferenceYAML(name, namespace, secretKey, remoteKey string) string {
	sr := &openchoreov1alpha1.SecretReference{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "SecretReference"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"openchoreo.dev/name": name,
			},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{
				Type: "Opaque",
			},
			Data: []openchoreov1alpha1.SecretDataSource{
				{
					SecretKey: secretKey,
					RemoteRef: openchoreov1alpha1.RemoteReference{
						Key: remoteKey,
					},
				},
			},
		},
	}
	return mustYAMLDocs(sr)
}

func componentAndWorkloadYAML() string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/name": componentName,
			},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: "deployment/worker",
			},
			AutoDeploy: true,
		},
	}

	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentName,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/name": componentName,
			},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentName,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{
					Image:   "busybox:1.36",
					Command: []string{"sh", "-c"},
					Args:    []string{"sleep 3600"},
					Env: []openchoreov1alpha1.EnvVar{
						{
							Key: "APP_USERNAME",
							ValueFrom: &openchoreov1alpha1.EnvVarValueFrom{
								SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{
									Name: "env-secret",
									Key:  "APP_USERNAME",
								},
							},
						},
					},
					Files: []openchoreov1alpha1.FileVar{
						{
							Key:       "password.txt",
							MountPath: "/etc/secrets",
							ValueFrom: &openchoreov1alpha1.EnvVarValueFrom{
								SecretKeyRef: &openchoreov1alpha1.SecretKeyRef{
									Name: "file-secret",
									Key:  "password.txt",
								},
							},
						},
					},
				},
			},
		},
	}

	return mustYAMLDocs(comp, workload)
}
