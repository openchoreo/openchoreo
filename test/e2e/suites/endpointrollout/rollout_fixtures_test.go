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

	componentTypeService = "deployment/service"

	// The backend serving now and the new version, identical except for the
	// text they answer with.
	compStable = "ep-stable"
	compCanary = "ep-canary"

	// The canary runs on one Endpoint; the A/B test and blue/green on the other.
	endpointName   = "greeter-api"
	endpointNameAB = "greeter-ab"

	gatewayHTTPPort = 29080

	testerLabel     = "app=ep-rollout-tester"
	testerContainer = "tester"
)

var runID = fmt.Sprintf("%d", time.Now().UnixNano())

var cpNs = "e2e-ep-rollout-" + runID

// endpointTypeName is unique per run: ClusterEndpointType is cluster-scoped and
// outlives the per-run control-plane namespace.
var endpointTypeName = "e2e-rollout-route-" + runID

// rulesRouteTemplate renders one HTTPRoute per environment on the external
// gateway with a rule per entry in ${rules}: each matched backend with its
// header conditions (a cookie arrives as one on the Cookie header), then the
// default rule splitting the rest by weight.
const rulesRouteTemplate = `{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "HTTPRoute",
  "metadata": {"name": "${metadata.name}", "namespace": "${metadata.namespace}", "labels": "${metadata.labels}"},
  "spec": {
    "parentRefs": [{"name": "${gateway.ingress.external.name}", "namespace": "${gateway.ingress.external.namespace}"}],
    "hostnames": ["${metadata.environmentName + '-' + metadata.endpointName + '.' + gateway.ingress.external.http.host}"],
    "rules": "${rules.map(r, {'matches': [{'path': {'type': 'PathPrefix', 'value': '/'}, 'headers': size(r.matches) > 0 ? r.matches[0].conditions.map(h, {'type': h.type, 'name': h.name, 'value': h.value}) : oc_omit()}], 'backendRefs': r.backends.filter(b, b.weight > 0).map(b, {'name': b.serviceName, 'port': b.port, 'weight': b.weight})})}"
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

// platformResourcesYAML is one environment, its pipeline and the project.
func platformResourcesYAML() string {
	return mustYAMLDocs(
		&openchoreov1alpha1.DeploymentPipeline{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
			ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: cpNs},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{
				PromotionPaths: []openchoreov1alpha1.PromotionPath{{
					SourceEnvironmentRef:  openchoreov1alpha1.EnvironmentRef{Name: envDev},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{},
				}},
			},
		},
		&openchoreov1alpha1.Environment{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{Name: envDev, Namespace: cpNs},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
			},
		},
		&openchoreov1alpha1.Project{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
			ObjectMeta: metav1.ObjectMeta{Name: projectName, Namespace: cpNs},
			Spec: openchoreov1alpha1.ProjectSpec{
				DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"},
				Type: openchoreov1alpha1.ProjectTypeRef{
					Kind: openchoreov1alpha1.ProjectTypeRefKindClusterProjectType,
					Name: "default",
				},
			},
		},
		// An unpinned ProjectReleaseBinding makes the data-plane cell namespace exist.
		&openchoreov1alpha1.ProjectReleaseBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ProjectReleaseBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: projectName + "-" + envDev, Namespace: cpNs},
			Spec: openchoreov1alpha1.ProjectReleaseBindingSpec{
				Owner:       openchoreov1alpha1.ProjectReleaseBindingOwner{ProjectName: projectName},
				Environment: envDev,
			},
		},
	)
}

// backendComponentYAML returns a Component and Workload answering with its own
// name on an "http" endpoint.
func backendComponentYAML(name string) string {
	return mustYAMLDocs(
		&openchoreov1alpha1.Component{
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
		},
		&openchoreov1alpha1.Workload{
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
						"http": {Protocol: openchoreov1alpha1.EndpointProtocolHTTP, Port: 8080},
					},
				},
			},
		},
	)
}

func clusterEndpointTypeYAML() string {
	return mustYAMLDocs(&openchoreov1alpha1.ClusterEndpointType{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "ClusterEndpointType"},
		ObjectMeta: metav1.ObjectMeta{Name: endpointTypeName},
		Spec: openchoreov1alpha1.ClusterEndpointTypeSpec{
			// A cookie group is a regular expression on the Cookie header; no
			// Mirror, so a mirror rollout is refused.
			Capabilities: []openchoreov1alpha1.EndpointCapability{
				openchoreov1alpha1.EndpointCapabilityMatch, openchoreov1alpha1.EndpointCapabilityRegularExpression,
			},
			Resources: []openchoreov1alpha1.ResourceTemplate{{
				ID:          "httproute-external",
				IncludeWhen: `${"external" in visibility}`,
				Template:    &runtime.RawExtension{Raw: []byte(rulesRouteTemplate)},
			}},
		},
	})
}

// endpointYAML returns an Endpoint serving the stable backend alone, deployed
// to development by autoDeploy.
func endpointYAML(name string) string {
	return mustYAMLDocs(&openchoreov1alpha1.Endpoint{
		TypeMeta:   metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Endpoint"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: cpNs},
		Spec: openchoreov1alpha1.EndpointSpec{
			Owner: openchoreov1alpha1.EndpointOwner{ProjectName: projectName},
			Type: openchoreov1alpha1.EndpointTypeRef{
				Kind: openchoreov1alpha1.EndpointTypeRefKindClusterEndpointType,
				Name: endpointTypeName,
			},
			Visibility: []openchoreov1alpha1.EndpointVisibility{openchoreov1alpha1.EndpointVisibilityExternal},
			AutoDeploy: true,
			BackendRefs: []openchoreov1alpha1.EndpointBackendRef{{
				Name:      "stable",
				Kind:      openchoreov1alpha1.EndpointBackendKindComponent,
				Component: &openchoreov1alpha1.EndpointComponentBackend{Name: compStable, Endpoint: "http"},
				Weight:    ptr.To(int32(100)),
			}},
		},
	})
}

func testerPodYAML(dpNamespace string) string {
	return mustYAMLDocs(&corev1.Pod{
		TypeMeta: metav1.TypeMeta{APIVersion: kubernetesAPIVerV1, Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ep-rollout-tester",
			Namespace: dpNamespace,
			Labels: map[string]string{
				"app":                       "ep-rollout-tester",
				"openchoreo.dev/project":    projectName,
				"openchoreo.dev/managed-by": "e2e-endpoint-rollout",
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name:    testerContainer,
			Image:   "busybox:1.36",
			Command: []string{"sleep", "3600"},
		}}},
	})
}

// routeURL is the address an Endpoint's rendered route serves in development.
func routeURL(endpoint string) string {
	return fmt.Sprintf("http://%s-%s.e2e-dp.local:%d/", envDev, endpoint, gatewayHTTPPort)
}
