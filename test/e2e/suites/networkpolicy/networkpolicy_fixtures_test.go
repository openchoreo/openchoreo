// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	dataPlane          = "e2e-np-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"
)

var npRunID = fmt.Sprintf("%d", time.Now().UnixNano())

// Control plane namespace names for test isolation.
var (
	cpNsAcme  = fmt.Sprintf("e2e-np-acme-%s", npRunID)
	cpNsBeta  = fmt.Sprintf("e2e-np-beta-%s", npRunID)
	nsExtSvc  = fmt.Sprintf("e2e-np-external-services-%s", npRunID)
	nsGateway = fmt.Sprintf("e2e-np-gateway-system-%s", npRunID)
)

func mustRawExtension(value any) *runtime.RawExtension {
	data, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal raw extension: %v", err))
	}
	return &runtime.RawExtension{Raw: data}
}

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

// dataPlaneYAML creates a namespaced DataPlane that test environments can reference.
// The clientCA value is injected at runtime after reading from the existing DataPlane.
func dataPlaneYAML(namespace, clientCAValue string) string {
	dp := &openchoreov1alpha1.DataPlane{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DataPlane"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataPlane,
			Namespace: namespace,
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			PlaneID: "default",
			ClusterAgent: openchoreov1alpha1.ClusterAgentConfig{
				ClientCA: openchoreov1alpha1.ValueFrom{Value: clientCAValue},
			},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "default"},
		},
	}
	return mustYAMLDocs(dp)
}

// cpNamespacesYAML defines control plane namespaces for the test.
func cpNamespacesYAML() string {
	acme := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNsAcme,
			Labels: map[string]string{
				"openchoreo.dev/controlplane-namespace": "true",
			},
		},
	}
	beta := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNsBeta,
			Labels: map[string]string{
				"openchoreo.dev/controlplane-namespace": "true",
			},
		},
	}
	return mustYAMLDocs(acme, beta)
}

// nonOCNamespacesYAML defines namespaces for resources outside the OC pipeline.
func nonOCNamespacesYAML() string {
	extSvc := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: nsExtSvc},
	}
	gateway := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: nsGateway},
	}
	return mustYAMLDocs(extSvc, gateway)
}

// nonOCPodsYAML defines raw pods that represent entities outside the OC pipeline.
func nonOCPodsYAML() string {
	extServicePod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-service",
			Namespace: nsExtSvc,
			Labels: map[string]string{
				"app": "ext-service",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "server",
			Image: "hashicorp/http-echo",
			Args:  []string{"-text=ext-service", "-listen=:8080"},
			Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
		}}},
	}

	extServiceSVC := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-service",
			Namespace: nsExtSvc,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "ext-service"},
			Ports: []corev1.ServicePort{{
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	gatewayProxyPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway-proxy",
			Namespace: nsGateway,
			Labels: map[string]string{
				"app": "gateway-proxy",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "client",
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}

	return mustYAMLDocs(extServicePod, extServiceSVC, gatewayProxyPod)
}

// platformResourcesYAML returns the platform resources for a given CP namespace.
// Each CP namespace gets a DeploymentPipeline, Environments, and Projects.
func platformResourcesYAML(cpNamespace string, environments, projects []string) string {
	promotionPaths := make([]openchoreov1alpha1.PromotionPath, 0)

	// Keep promotionPaths non-empty. Empty paths prevent component reconciliation
	// and break project/deployment pipeline finalization in teardown.
	if len(environments) == 0 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: "development",
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name:             "development",
				RequiresApproval: false,
			}},
		})
	} else if len(environments) == 1 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: environments[0],
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name:             environments[0],
				RequiresApproval: false,
			}},
		})
	} else {
		for i := 0; i < len(environments)-1; i++ {
			promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
				SourceEnvironmentRef: environments[i],
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
					Name:             environments[i+1],
					RequiresApproval: false,
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
					Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
					Name: dataPlane,
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
			Spec: openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: "default"},
		})
	}

	return mustYAMLDocs(docs...)
}

// componentTypesYAML returns minimal ComponentType definitions for testing.
// e2e-service: renders Deployment + Service (no HTTPRoute/ConfigMap/ExternalSecret).
// e2e-worker: renders Deployment only (for busybox client pods).
func componentTypesYAML(cpNamespace string) string {
	deploymentTemplate := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "${metadata.name}",
			"namespace": "${metadata.namespace}",
			"labels":    "${metadata.labels}",
		},
		"spec": map[string]any{
			"replicas": "${envOverrides.replicas}",
			"selector": map[string]any{
				"matchLabels": "${metadata.podSelectors}",
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": "${metadata.podSelectors}",
				},
				"spec": map[string]any{
					"containers": []any{map[string]any{
						"name":    "main",
						"image":   "${workload.container.image}",
						"command": "${has(workload.container.command) ? workload.container.command : oc_omit()}",
						"args":    "${has(workload.container.args) ? workload.container.args : oc_omit()}",
					}},
				},
			},
		},
	}

	serviceTemplate := map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]any{
			"name":      "${metadata.componentName}",
			"namespace": "${metadata.namespace}",
			"labels":    "${metadata.labels}",
		},
		"spec": map[string]any{
			"type":     "ClusterIP",
			"selector": "${metadata.podSelectors}",
			"ports":    "${workload.toServicePorts()}",
		},
	}

	e2eServiceCT := &openchoreov1alpha1.ComponentType{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ComponentType"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-service",
			Namespace: cpNamespace,
		},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Validations: []openchoreov1alpha1.ValidationRule{{
				Rule:    "${size(workload.endpoints) > 0}",
				Message: "e2e-service must have at least one endpoint.",
			}},
			Schema: openchoreov1alpha1.ComponentTypeSchema{
				Parameters:   mustRawExtension(map[string]any{}),
				EnvOverrides: mustRawExtension(map[string]any{"replicas": "integer | default=1"}),
			},
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{ID: "deployment", Template: mustRawExtension(deploymentTemplate)},
				{ID: "service", IncludeWhen: "${size(workload.endpoints) > 0}", Template: mustRawExtension(serviceTemplate)},
			},
		},
	}

	e2eWorkerCT := &openchoreov1alpha1.ComponentType{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ComponentType"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-worker",
			Namespace: cpNamespace,
		},
		Spec: openchoreov1alpha1.ComponentTypeSpec{
			WorkloadType: "deployment",
			Validations: []openchoreov1alpha1.ValidationRule{{
				Rule:    "${size(workload.endpoints) == 0}",
				Message: "e2e-worker must not have endpoints.",
			}},
			Schema: openchoreov1alpha1.ComponentTypeSchema{
				Parameters:   mustRawExtension(map[string]any{}),
				EnvOverrides: mustRawExtension(map[string]any{"replicas": "integer | default=1"}),
			},
			Resources: []openchoreov1alpha1.ResourceTemplate{
				{ID: "deployment", Template: mustRawExtension(deploymentTemplate)},
			},
		},
	}

	return mustYAMLDocs(e2eServiceCT, e2eWorkerCT)
}

// componentYAML returns a Component + Workload pair.
func componentYAML(cpNamespace, project, name, componentType, image string, args []string, endpoints map[string]endpointDef) string {
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

	if len(endpoints) > 0 {
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

	return mustYAMLDocs(comp, workload)
}

type endpointDef struct {
	epType     string
	port       int
	visibility []string
}

// releaseBindingYAML creates a ReleaseBinding that deploys an existing ComponentRelease
// to a specific environment. Used to promote a component to staging without autoDeploy.
func releaseBindingYAML(cpNamespace, project, component, releaseName, environment string) string {
	rb := &openchoreov1alpha1.ReleaseBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", component, environment),
			Namespace: cpNamespace,
		},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner: openchoreov1alpha1.ReleaseBindingOwner{
				ProjectName:   project,
				ComponentName: component,
			},
			Environment: environment,
			ReleaseName: releaseName,
			State:       openchoreov1alpha1.ReleaseStateActive,
		},
	}
	return mustYAMLDocs(rb)
}
