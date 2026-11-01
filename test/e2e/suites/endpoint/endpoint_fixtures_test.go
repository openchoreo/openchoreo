// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
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

	// The two backends: identical except for the text they answer with, so a
	// response identifies which one served it.
	compStable = "ep-stable"
	compCanary = "ep-canary"

	endpointName = "greeter-api"

	// The e2e ClusterDataPlane's external gateway. Its http listener serves
	// *.e2e-dp.local, which the e2e CoreDNS rewrites to the gateway.
	gatewayHTTPPort = 29080

	testerLabel     = "app=ep-tester"
	testerContainer = "tester"
)

var runID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = fmt.Sprintf("e2e-ep-%s", runID)

// endpointTypeName is unique per run: ClusterEndpointType is cluster-scoped and
// outlives the per-run control-plane namespace.
var endpointTypeName = "e2e-weighted-route-" + runID

// weightedRouteTemplate renders one HTTPRoute per environment on the external
// gateway, splitting traffic across every resolved backend by weight.
const weightedRouteTemplate = `{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "HTTPRoute",
  "metadata": {"name": "${metadata.name}", "namespace": "${metadata.namespace}", "labels": "${metadata.labels}"},
  "spec": {
    "parentRefs": [{"name": "${gateway.ingress.external.name}", "namespace": "${gateway.ingress.external.namespace}"}],
    "hostnames": ["${metadata.environmentName + '-' + metadata.endpointName + '.' + gateway.ingress.external.http.host}"],
    "rules": [{
      "matches": [{"path": {"type": "PathPrefix", "value": "/"}}],
      "backendRefs": "${backends.map(b, {'name': b.serviceName, 'port': b.port, 'weight': b.weight})}"
    }]
  }
}`

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
	return mustYAMLDocs(&corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   cpNs,
			Labels: map[string]string{"openchoreo.dev/control-plane": "true"},
		},
	})
}

func platformResourcesYAML() string {
	pipeline := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: cpNs},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{{
				SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: envDev},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{Name: envStaging}},
			}},
		},
	}
	docs := []any{pipeline}
	for _, envName := range []string{envDev, envStaging} {
		docs = append(docs, &openchoreov1alpha1.Environment{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{Name: envName, Namespace: cpNs},
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
		ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: cpNs},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
			Type: openchoreov1alpha1.ProjectTypeRef{
				Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType,
				Name: "default",
			},
		},
	})
	// Unpinned ProjectReleaseBindings make the data-plane cell namespaces exist.
	for _, envName := range []string{envDev, envStaging} {
		docs = append(docs, &openchoreov1alpha1.ProjectReleaseBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-" + envName, Namespace: cpNs},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
				Environment: envName,
			},
		})
	}
	return mustYAMLDocs(docs...)
}

// backendComponentYAML returns a Component and Workload answering with its own
// name on an "http" endpoint. The endpoint declares no visibility: the
// Endpoint carries it, and the network policy the Endpoint renders with its
// routes is what admits the gateway to the component's pods. The component's
// own policy admits only its project.
func backendComponentYAML(name string) string {
	comp := &openchoreov1alpha1.Component{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Component"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cpNs},
		Spec: openchoreov1alpha1.ComponentSpec{
			Owner: openchoreov1alpha1.ComponentOwner{ProjectName: projectName},
			ComponentType: openchoreov1alpha1.ComponentTypeRef{
				Kind: openchoreov1alpha1.ComponentTypeRefKindClusterComponentType,
				Name: componentTypeService,
			},
			AutoDeploy: true,
		},
	}
	workload := &openchoreov1alpha1.Workload{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Workload"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cpNs},
		Spec: openchoreov1alpha1.WorkloadSpec{
			Owner: openchoreov1alpha1.WorkloadOwner{ProjectName: projectName, ComponentName: name},
			WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
				Container: openchoreov1alpha1.Container{
					Image: "hashicorp/http-echo",
					Args:  []string{"-text=" + name, "-listen=:8080"},
				},
				Endpoints: map[string]openchoreov1alpha1.WorkloadEndpoint{
					// No visibility on the workload: the Endpoint carries it, and
					// its own network policy admits the gateway. The ComponentType
					// renders no route of its own.
					"http": {
						Protocol: openchoreov1alpha1.EndpointProtocolHTTP,
						Port:     8080,
					},
				},
			},
		},
	}
	return mustYAMLDocs(comp, workload)
}

func releaseBindingYAML(component, releaseName, environment string) string {
	return mustYAMLDocs(&openchoreov1alpha1.ReleaseBinding{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{Name: component + "-" + environment, Namespace: cpNs},
		Spec: openchoreov1alpha1.ReleaseBindingSpec{
			Owner:       openchoreov1alpha1.ReleaseBindingOwner{ProjectName: projectName, ComponentName: component},
			ReleaseName: releaseName,
			Environment: environment,
		},
	})
}

func clusterEndpointTypeYAML() string {
	return mustYAMLDocs(&openchoreov1alpha1.ClusterEndpointType{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ClusterEndpointType"},
		ObjectMeta: metav1.ObjectMeta{Name: endpointTypeName},
		Spec: openchoreov1alpha1.ClusterEndpointTypeSpec{
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:          "httproute-external",
				IncludeWhen: `${"external" in visibility}`,
				Template:    &runtime.RawExtension{Raw: []byte(weightedRouteTemplate)},
			}},
			// Built from the same gateway data as the route's hostname, so the
			// test can call the endpoint on the URL the binding reports.
			URLs: &openchoreov1alpha1.EndpointTypeURLs{External: &openchoreov1alpha1.EndpointURLTemplate{
				Scheme:   "http",
				Host:     "${metadata.environmentName}-${metadata.endpointName}.${gateway.ingress.external.http.host}",
				Port:     "${gateway.ingress.external.http.port}",
				Path:     "/",
				Security: &openchoreov1alpha1.EndpointSecurityTemplate{Scheme: "none"},
			}},
		},
	})
}

// endpointYAML returns the Endpoint splitting traffic between the two
// backend components with the given weights.
func endpointYAML(stableWeight, canaryWeight int32) string {
	backend := func(name, component string, weight int32) openchoreov1alpha1.EndpointBackendRef {
		return openchoreov1alpha1.EndpointBackendRef{
			Name:      name,
			Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
			Component: &openchoreov1alpha1.EndpointComponentBackend{Name: component, Endpoint: "http"},
			Weight:    ptr.To(weight),
		}
	}
	return mustYAMLDocs(&openchoreov1alpha1.Endpoint{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Endpoint"},
		ObjectMeta: metav1.ObjectMeta{Name: endpointName, Namespace: cpNs},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: projectName},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
				Name: endpointTypeName,
			},
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			BackendRefs: []openchoreov1alpha1.EndpointBackendRef{
				backend("stable", compStable, stableWeight),
				backend("canary", compCanary, canaryWeight),
			},
		},
	})
}

func endpointReleaseBindingYAML(environment, releaseName string) string {
	return mustYAMLDocs(&openchoreov1alpha1.EndpointReleaseBinding{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "EndpointReleaseBinding"},
		ObjectMeta: metav1.ObjectMeta{Name: endpointName + "-" + environment, Namespace: cpNs},
		Spec: openchoreov1alpha1.EndpointReleaseBindingSpec{
			Owner:           openchoreov1alpha1.EndpointReleaseBindingOwner{ProjectName: projectName, EndpointName: endpointName},
			Environment:     environment,
			EndpointRelease: releaseName,
		},
	})
}

func testerPodYAML(dpNamespace string) string {
	return mustYAMLDocs(&corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "ep-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-endpoint",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    testerContainer,
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	})
}

// routeURL is the address the rendered route serves in an environment.
func routeURL(environment string) string {
	return fmt.Sprintf("http://%s-%s.e2e-dp.local:%d/", environment, endpointName, gatewayHTTPPort)
}
