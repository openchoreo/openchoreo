// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strconv"
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

	projectName = "obs-proj"
	envDev      = "development"
	envStaging  = "staging"

	componentGreeter     = "obs-greeter"
	releaseBindingSuffix = "-" + envDev

	servicePort = 9090

	// imageGreeter is the public OpenChoreo sample image with a
	// `/greeter/greet` endpoint and stdout logging.
	imageGreeter = "ghcr.io/openchoreo/samples/greeter-service@sha256:5c67732c99ac3505dbab14c7ec92c33be57904420d62812694c64b56c5f92d40"

	// curlImage is the in-cluster pod the framework execs queries through.
	// curl is available in the curlimages/curl image and the image is tiny
	// (~5MB) — a good fit for the suite's load-generation + observer-query
	// needs. Pinned to avoid a "latest" surprise.
	curlImage     = "curlimages/curl:8.10.1"
	curlPodLabel  = "app=obs-tester"
	curlContainer = "tester"
)

var obsRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-obs-%s", obsRunID)

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
		},
	}
	docs := []any{pipeline}
	docs = append(docs, envs...)
	docs = append(docs, proj)
	return mustYAMLDocs(docs...)
}

// greeterComponentYAML returns a service-flavour Component + Workload that
// exposes the greeter sample on `servicePort`. Logs land on stdout, which
// the cluster's logs-adapter ships into OpenSearch under the rendered DP
// namespace + the component's pod labels — which is exactly what the
// observer's componentSearchScope query needs to find them.
func greeterComponentYAML() string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      componentGreeter,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": componentGreeter},
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
			Name:      componentGreeter,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": componentGreeter},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   projectName,
				ComponentName: componentGreeter,
			},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					"http": {
						Type:       openchoreov1alpha1.EndpointType("HTTP"),
						Port:       int32(servicePort),
						Visibility: []openchoreov1alpha1.EndpointVisibility{"project"},
					},
				},
				Container: openchoreov1alpha1.Container{
					Image: imageGreeter,
					Args:  []string{"--port", strconv.Itoa(servicePort)},
				},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}

// curlPodYAML returns a curl-enabled tester pod the framework execs through
// to (a) generate HTTP traffic against the greeter and (b) call the observer
// query API. Same pattern as wt-tester from the workloadtypes suite, but
// using curlimages/curl so we get a TLS-capable curl (the observer is plain
// HTTP, but using one image keeps the suites consistent).
func curlPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "obs-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "obs-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-observability",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    curlContainer,
			Image:   curlImage,
			Command: []string{"sleep", "3600"},
		}}},
	}
	return mustYAMLDocs(pod)
}
