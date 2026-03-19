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
)

var connRunID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-conn-%s", connRunID)

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

func platformResourcesYAML(cpNamespace string, environments, projects []string) string {
	promotionPaths := make([]openchoreov1alpha1.PromotionPath, 0)

	if len(environments) == 0 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "development"},
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name: "development",
			}},
		})
	} else if len(environments) == 1 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: environments[0]},
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name: environments[0],
			}},
		})
	} else {
		for i := 0; i < len(environments)-1; i++ {
			promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: environments[i]},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
					Name: environments[i+1],
				}},
			})
		}
	}

	docs := []any{
		&openchoreov1alpha1.DeploymentPipeline{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": "default",
				},
			},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{PromotionPaths: promotionPaths},
		},
	}

	for _, env := range environments {
		docs = append(docs, &openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      env,
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": env,
				},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
				IsProduction: false,
			},
		})
	}

	for _, proj := range projects {
		docs = append(docs, &openchoreov1alpha1.Project{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      proj,
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": proj,
				},
			},
			Spec: openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"}},
		})
	}

	return mustYAMLDocs(docs...)
}

type endpointDef struct {
	epType     string
	port       int
	visibility []string
}

type connectionDef struct {
	project    string
	component  string
	endpoint   string
	visibility string
	envURL     string
	envHost    string
	envPort    string
}

// populateEndpoints sets workload endpoints from the given endpoint definitions.
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
		workload.Spec.Endpoints[epName] = openchoreov1alpha1.WorkloadEndpoint{
			Type:       openchoreov1alpha1.EndpointType(ep.epType),
			Port:       int32(ep.port),
			Visibility: visibility,
		}
	}
}

// populateConnections sets workload connections from the given connection definitions.
func populateConnections(workload *openchoreov1alpha1.Workload, connections []connectionDef) {
	if len(connections) == 0 {
		return
	}
	endpointConns := make([]openchoreov1alpha1.WorkloadConnection, 0, len(connections))
	for _, conn := range connections {
		wc := openchoreov1alpha1.WorkloadConnection{
			Project:    conn.project,
			Component:  conn.component,
			Name:       conn.endpoint,
			Visibility: openchoreov1alpha1.EndpointVisibility(conn.visibility),
			EnvBindings: openchoreov1alpha1.ConnectionEnvBindings{
				Address: conn.envURL,
				Host:    conn.envHost,
				Port:    conn.envPort,
			},
		}
		endpointConns = append(endpointConns, wc)
	}
	workload.Spec.Dependencies = &openchoreov1alpha1.WorkloadDependencies{
		Endpoints: endpointConns,
	}
}

// componentWithConnectionsYAML returns a Component + Workload pair, with optional connections.
func componentWithConnectionsYAML(
	cpNamespace, project, name, componentType, image string,
	args []string,
	endpoints map[string]endpointDef,
	connections []connectionDef,
) string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNamespace,
			Labels: map[string]string{
				"openchoreo.dev/name": name,
			},
		},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: project},
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
			Namespace: cpNamespace,
			Labels: map[string]string{
				"openchoreo.dev/name": name,
			},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   project,
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
	populateConnections(workload, connections)

	return mustYAMLDocs(comp, workload)
}

// workloadOnlyYAML returns only a Workload resource (for updating an existing component's workload).
func workloadOnlyYAML(
	cpNamespace, project, name, image string,
	args []string,
	endpoints map[string]endpointDef,
	connections []connectionDef,
) string {
	workload := &openchoreov1alpha1.Workload{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cpNamespace,
			Labels: map[string]string{
				"openchoreo.dev/name": name,
			},
		},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{
				ProjectName:   project,
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
	populateConnections(workload, connections)

	return mustYAMLDocs(workload)
}
