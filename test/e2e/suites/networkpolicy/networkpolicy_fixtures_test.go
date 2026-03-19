// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	clusterDataPlane   = "e2e-shared"
	openChoreoAPIVer   = "openchoreo.dev/v1alpha1"
	kubernetesAPIVerV1 = "v1"
)

var npRunID = fmt.Sprintf("%d", time.Now().UnixNano())

// OC namespace names for test isolation.
var (
	cpNsAcme  = fmt.Sprintf("e2e-np-acme-%s", npRunID)
	cpNsBeta  = fmt.Sprintf("e2e-np-beta-%s", npRunID)
	nsExtSvc  = fmt.Sprintf("e2e-np-external-services-%s", npRunID)
	nsGateway = fmt.Sprintf("e2e-np-gateway-system-%s", npRunID)
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

// cpNamespacesYAML defines OC namespaces (k8s namespaces labeled as openchoreo.dev/control-plane) for the test.
func cpNamespacesYAML() string {
	acme := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNsAcme,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
			},
		},
	}
	beta := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: cpNsBeta,
			Labels: map[string]string{
				"openchoreo.dev/control-plane": "true",
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
				"app":                             "gateway-proxy",
				"openchoreo.dev/system-component": "gateway",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "client",
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}

	extClientPod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ext-client",
			Namespace: nsExtSvc,
			Labels: map[string]string{
				"app": "ext-client",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    "client",
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	}

	return mustYAMLDocs(extServicePod, extServiceSVC, extClientPod, gatewayProxyPod)
}

// platformResourcesYAML returns the platform resources for a given OC namespace.
// Each OC namespace gets a DeploymentPipeline, Environments, and Projects.
func platformResourcesYAML(cpNamespace string, environments, projects []string) string {
	promotionPaths := make([]openchoreov1alpha1.PromotionPath, 0)

	// Keep promotionPaths non-empty. Empty paths prevent component reconciliation
	// and break project/deployment pipeline finalization in teardown.
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

// unprotectedPodYAML creates a bare Pod + Service inside a data plane namespace.
// No NetworkPolicy selects this pod, so all ingress is allowed by default.
// This isolates egress-only denial: if the source's egress policy is working,
// cross-CP or cross-env traffic to this pod is still blocked.
func unprotectedPodYAML(namespace, name string) string {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:  "server",
			Image: "hashicorp/http-echo",
			Args:  []string{fmt.Sprintf("-text=%s", name), "-listen=:8080"},
			Ports: []corev1.ContainerPort{{ContainerPort: 8080}},
		}}},
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Port:       8080,
				TargetPort: intstr.FromInt(8080),
			}},
		},
	}

	return mustYAMLDocs(pod, svc)
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
