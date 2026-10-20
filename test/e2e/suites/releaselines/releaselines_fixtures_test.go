// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"

	projectName = "rl-proj"
	envDev      = "development"
	envStaging  = "staging"

	// lineComponentType allows release lines. The e2e setup installs it from samples/release-lines.
	lineComponentType = "deployment/release-line-service"
	// plainComponentType does not allow release lines.
	plainComponentType = "deployment/service"

	componentGreeter = "greeter"
	componentEcho    = "echo"
	componentPlain   = "plain"

	// Line names are unique within a namespace, so every line in the suite has its own name.
	lineStable   = "stable"
	lineCanary   = "canary"
	lineEchoMain = "echo-main"

	healthTrait   = "health"
	scalerTrait   = "scaler"
	greeterImage  = "ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40"
	greeterPort   = 9090
	greeterPath   = "/greeter/greet"
	testerLabel   = "app=rl-tester"
	testerName    = "rl-tester"
	testerContain = "tester"

	labelComponent   = "openchoreo.dev/component"
	labelReleaseLine = "openchoreo.dev/release-line"
)

// clusterDataPlane is the e2e cluster's data plane. E2E_CLUSTER_DATAPLANE overrides it to run the
// suite against another cluster, such as a local k3d install.
var clusterDataPlane = envOr("E2E_CLUSTER_DATAPLANE", "e2e-shared")

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var rlRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-rl-%s", rlRunID)

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

func rawJSON(v any) *runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal raw extension: %v", err))
	}
	return &runtime.RawExtension{Raw: data}
}

func objectMeta(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: cpNs,
		Labels:    map[string]string{"openchoreo.dev/name": name},
	}
}

// scopedName mirrors how the controllers name the objects of a release line.
func scopedName(component, line, suffix string) string {
	return fmt.Sprintf("%s-%s-%s", component, line, suffix)
}

func cpNamespaceYAML() string {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   cpNs,
			Labels: map[string]string{"openchoreo.dev/control-plane": "true"},
		},
	}
	return mustYAMLDocs(ns)
}

// platformResourcesYAML creates a development → staging pipeline and binds the project to both
// environments, so each gets a data plane namespace.
func platformResourcesYAML() string {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: objectMeta("default"),
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: envStaging}},
			}},
		},
	}
	docs := []any{pipeline}
	for _, env := range []string{envDev, envStaging} {
		docs = append(docs, &openchoreov1alpha1.Environment{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: objectMeta(env),
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
			},
		})
	}
	docs = append(docs, &openchoreov1alpha1.Project{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
		ObjectMeta: objectMeta(projectName),
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type: openchoreov1alpha1.ProjectTypeRef{
				Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType,
				Name: "default",
			},
		},
	})
	for _, env := range []string{envDev, envStaging} {
		docs = append(docs, &openchoreov1alpha1.ProjectReleaseBinding{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      projectName + "-" + env,
				Namespace: cpNs,
				Labels: map[string]string{
					"openchoreo.dev/project":     projectName,
					"openchoreo.dev/environment": env,
				},
			},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
				Environment: env,
			},
		})
	}
	return mustYAMLDocs(docs...)
}

func lineComponent(name, componentType string, traits []openchoreov1alpha1.ComponentTrait) *openchoreov1alpha1.Component {
	return &openchoreov1alpha1.Component{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: objectMeta(name),
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: componentType,
			},
			AutoDeploy:          true,
			ReleaseLinesEnabled: true,
			Parameters:          rawJSON(map[string]any{"logLevel": "info"}),
			Traits:              traits,
		},
	}
}

func releaseLine(name, component string) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ReleaseLine"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNs,
		},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{ProjectName: projectName, ComponentName: component},
		},
	}
}

// lineWorkload is the greeter Workload of one release line. An empty line gives a Workload without
// one, which a component with release lines enabled rejects.
func lineWorkload(component, line string) *openchoreov1alpha1.Workload {
	name := component + "-workload"
	if line != "" {
		name = scopedName(component, line, "workload")
	}
	return &openchoreov1alpha1.Workload{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: objectMeta(name),
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: component,
				ReleaseLine:   line,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {
						Type:       openchoreov1alpha1.EndpointType("HTTP"),
						Port:       greeterPort,
						Visibility: []openchoreov1alpha1.EndpointVisibility{"project", "external"},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: greeterImage,
					Args:  []string{"--port", fmt.Sprint(greeterPort)},
				},
			},
		},
	}
}

// greeterYAML is a component with release lines enabled and a readiness probe trait, plus two
// lines. stable inherits everything. canary logs at debug, adds a liveness probe to the
// component's health trait, and adds an autoscaler trait of its own.
func greeterYAML() string {
	health := openchoreov1alpha1.ComponentTrait{
		Kind:         openchoreov1alpha1.TraitRefKindClusterTrait,
		Name:         "health-probes",
		InstanceName: healthTrait,
		Parameters: rawJSON(map[string]any{
			"readiness": map[string]any{
				"enabled": true,
				"handler": map[string]any{"tcpSocket": map[string]any{"port": greeterPort}},
			},
		}),
	}
	comp := lineComponent(componentGreeter, lineComponentType, []openchoreov1alpha1.ComponentTrait{health})

	canary := releaseLine(lineCanary, componentGreeter)
	canary.Spec.Parameters = rawJSON(map[string]any{"logLevel": "debug"})
	canary.Spec.Traits = []openchoreov1alpha1.ComponentTrait{
		{
			Kind:         openchoreov1alpha1.TraitRefKindClusterTrait,
			Name:         "health-probes",
			InstanceName: healthTrait,
			Parameters: rawJSON(map[string]any{
				"liveness": map[string]any{
					"enabled": true,
					"handler": map[string]any{"tcpSocket": map[string]any{"port": greeterPort}},
				},
			}),
		},
		{
			Kind:         openchoreov1alpha1.TraitRefKindClusterTrait,
			Name:         "horizontal-pod-autoscaler",
			InstanceName: scalerTrait,
			Parameters:   rawJSON(map[string]any{"replicas": map[string]any{"min": 1, "max": 2}}),
		},
	}

	return mustYAMLDocs(
		comp,
		releaseLine(lineStable, componentGreeter),
		canary,
		lineWorkload(componentGreeter, lineStable),
		lineWorkload(componentGreeter, lineCanary),
	)
}

// promotionYAML binds a release of a line to staging.
func promotionYAML(component, line, release string) string {
	rb := &openchoreov1alpha1.ReleaseBinding{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ReleaseBinding"},
		ObjectMeta: objectMeta(scopedName(component, line, envStaging)),
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   projectName,
				ComponentName: component,
				ReleaseLine:   line,
			},
			Environment: envStaging,
			ReleaseName: release,
		},
	}
	return mustYAMLDocs(rb)
}

// testerPodYAML is a busybox pod in the data plane namespace, used to call the lines' services.
func testerPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testerName,
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       testerName,
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-releaselines",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    testerContain,
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}
