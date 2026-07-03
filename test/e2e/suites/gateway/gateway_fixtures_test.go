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

	projectName = "default"
	envDev      = "development"
	envStaging  = "staging"

	componentTypeService = "deployment/service"

	testerLabel     = "app=gw-tester"
	testerContainer = "tester"

	gwDefaultNS    = "openchoreo-data-plane"
	gwInternalName = "gateway-internal"

	// Staging environment gateway override.
	stagingGWHost = "e2e-gw-staging.local"
	stagingGWPort = 18080
)

type endpointDef struct {
	epType     string
	port       int
	visibility []string
	basePath   string
}

var runID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-gw-%s", runID)

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

	envDevObj := &openchoreov1alpha1.Environment{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      envDev,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": envDev},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
				Name: clusterDataPlane,
			},
		},
	}

	// Staging environment with gateway override pointing to gateway-internal.
	envStgObj := &openchoreov1alpha1.Environment{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      envStaging,
			Namespace: cpNs,
			Labels:    map[string]string{"openchoreo.dev/name": envStaging},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
				Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
				Name: clusterDataPlane,
			},
			Gateway: openchoreov1alpha1.GatewaySpec{
				Ingress: &openchoreov1alpha1.GatewayNetworkSpec{
					External: &openchoreov1alpha1.GatewayEndpointSpec{
						Name:      gwInternalName,
						Namespace: gwDefaultNS,
						HTTP: &openchoreov1alpha1.GatewayListenerSpec{
							Port: stagingGWPort,
							Host: stagingGWHost,
						},
					},
				},
			},
		},
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

	return mustYAMLDocs(pipeline, envDevObj, envStgObj, proj)
}

func componentYAML(name, image string, args []string, endpoints map[string]endpointDef, traits []openchoreov1alpha1.ComponentTrait) string {
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
				Name: componentTypeService,
			},
			AutoDeploy: true,
			Traits:     traits,
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
					Args:  args,
				},
			},
		},
	}

	populateEndpoints(workload, endpoints)
	return mustYAMLDocs(comp, workload)
}

func populateEndpoints(workload *openchoreov1alpha1.Workload, endpoints map[string]endpointDef) {
	if len(endpoints) == 0 {
		return
	}
	workload.Spec.Endpoints = make(map[string]openchoreov1alpha1.WorkloadEndpoint, len(endpoints))
	for epName, ep := range endpoints {
		visibility := make([]openchoreov1alpha1.EndpointVisibility, 0, len(ep.visibility))
		for _, v := range ep.visibility {
			visibility = append(visibility, openchoreov1alpha1.EndpointVisibility(v))
		}
		we := openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointType(ep.epType),
			Port:       int32(ep.port),
			Visibility: visibility,
		}
		if ep.basePath != "" {
			we.BasePath = ep.basePath
		}
		workload.Spec.Endpoints[epName] = we
	}
}

func componentReleaseBindingYAML(component, releaseName, environment string) string {
	rb := map[string]any{
		"apiVersion": openChoreoAPIVer,
		"kind":       "ComponentReleaseBinding",
		"metadata": map[string]any{
			"name":      component + "-" + environment,
			"namespace": cpNs,
		},
		"spec": map[string]any{
			"owner": map[string]any{
				"projectName":   projectName,
				"componentName": component,
			},
			"releaseName": releaseName,
			"environment": environment,
		},
	}
	data, err := yaml.Marshal(rb)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal ComponentReleaseBinding: %v", err))
	}
	return strings.TrimSpace(string(data))
}

func testerPodYAML(dpNamespace string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "gw-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-gateway",
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
