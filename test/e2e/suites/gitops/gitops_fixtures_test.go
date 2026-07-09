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

	projectName = "gitops-proj"
	envDev      = "development"
	envStaging  = "staging"

	giteaNamespace = "e2e-gitea-gitops"
	gitopsRepo     = "gitops-platform"

	componentSingle = "echo-svc"
	componentBulkA  = "bulk-a"
	componentBulkB  = "bulk-b"
	componentBulkC  = "bulk-c"

	imageInitial = "hashicorp/http-echo:1.0.0"
	imageUpdated = "hashicorp/http-echo:0.2.3"

	releaseBindingSuffix = "-" + envDev
)

var (
	gitopsRunID = fmt.Sprintf("%d", time.Now().UnixNano())
	cpNs        = fmt.Sprintf("e2e-gitops-%s", gitopsRunID)
	fluxNs      = "flux-system"
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

// platformResourcesYAML renders the CP namespace's pipeline + environments +
// project. Flux applies these (along with the per-spec components) but the
// project + environments live in the same YAML tree so a single Kustomization
// covers the lot.
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
	envs := []any{}
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
	// ProjectReleaseBinding deploys the project to the development environment,
	// creating its cell (DP) namespace. spec.projectRelease is left unset; the
	// Project controller seeds it once the first ProjectRelease is cut.
	binding := &openchoreov1alpha1.ProjectReleaseBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName + "-" + envDev,
			Namespace: cpNs,
			Labels: map[string]string{
				"openchoreo.dev/project":     projectName,
				"openchoreo.dev/environment": envDev,
			},
		},
		Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
			Environment: envDev,
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj, binding)
	return mustYAMLDocs(docs...)
}

// componentWithImageYAML builds a Component + Workload pair for a deployment-
// style component, with the supplied image. Identical shape to the workloadtypes
// suite so the two suites read alike — the components here are deliberately
// thin so Flux is the interesting variable.
func componentWithImageYAML(name, image, echoText string) string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": name},
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
	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": name},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: name,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {
						Type:       openchoreov1alpha1.EndpointType("HTTP"),
						Port:       8080,
						Visibility: []openchoreov1alpha1.EndpointVisibility{"project"},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: image,
					Args:  []string{"-listen=:8080", "-text=" + echoText},
				},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}
