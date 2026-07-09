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

	projectName = "wt-proj"
	envDev      = "development"
	envStaging  = "staging"

	componentService     = "greeter"
	componentWebApp      = "react-spa"
	componentScheduled   = "issue-reporter"
	releaseBindingSuffix = "-" + envDev

	servicePort = 9090
	webAppPort  = 8080

	testerLabel     = "app=wt-tester"
	testerContainer = "tester"
)

var wtRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-wt-%s", wtRunID)

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
	// Pipeline must have a root environment — at least one env that is a
	// source but not a target. We deploy to `development` (the root) and add
	// `staging` as a downstream-only env to satisfy that invariant; staging
	// is never actually exercised by this suite.
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
	// ProjectReleaseBinding deploys the project to the development environment,
	// creating its cell (DP) namespace. spec.projectRelease is left unset; the
	// Project controller seeds it once the first ProjectRelease is cut.
	binding := &openchoreov1alpha1.ProjectReleaseBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectName + releaseBindingSuffix,
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

// componentWithImageYAML returns a Component + Workload pair for a deployment-style
// component type (service / web-application). Port and image are caller-provided.
func componentWithImageYAML(name, componentType, image string, port int, args []string) string {
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
				Name: componentType,
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
						Type: openchoreov1alpha1.EndpointType("HTTP"),
						Port: int32(port),
						// project keeps the in-cluster ClusterIP probe alive;
						// external renders an HTTPRoute on the data plane's
						// external kgateway listener so the same workload is
						// reachable through the public URL too.
						Visibility: []openchoreov1alpha1.EndpointVisibility{"project", "external"},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: image,
					Args:  args,
				},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}

// scheduledTaskComponentYAML returns a Component + Workload + ReleaseBinding for
// the cronjob/scheduled-task ClusterComponentType. The ReleaseBinding sets a
// minute-frequency schedule so the test can observe a scheduled Job within ~60s.
func scheduledTaskComponentYAML(name, image string) string {
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
				Name: "cronjob/scheduled-task",
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
				Container: openchoreov1alpha1.Container{
					Image: image,
				},
			},
		},
	}
	// ReleaseBinding override forces a 1-minute schedule so the test does not
	// have to wait the sample's default 5 minutes for `lastScheduleTime`.
	rb := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "ReleaseBinding",
		"metadata": map[string]any{
			"name":      name + releaseBindingSuffix,
			"namespace": cpNs,
		},
		"spec": map[string]any{
			"owner": map[string]any{
				"projectName":   projectName,
				"componentName": name,
			},
			"environment": envDev,
			"componentTypeEnvironmentConfigs": map[string]any{
				"schedule": "* * * * *",
			},
		},
	}
	return mustYAMLDocs(comp, workload, rb)
}

// testerPodYAML returns a busybox pod that sleeps forever, used as the source
// of in-cluster wget probes against project-visibility workloads. Deployed in
// the data plane namespace so it shares the project network policy scope.
func testerPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "wt-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "wt-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-workloadtypes",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    testerContainer,
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}
