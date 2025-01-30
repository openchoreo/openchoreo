/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubernetes

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/wso2-enterprise/choreo-cp-declarative-api/api/v1"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/controller"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/dataplane"
	dpkubernetes "github.com/wso2-enterprise/choreo-cp-declarative-api/internal/dataplane/kubernetes"
	"github.com/wso2-enterprise/choreo-cp-declarative-api/internal/ptr"
)

// Gateway Types
const (
	gatewayExternal = "gateway-external"
	gatewayInternal = "gateway-internal"
)

type httpRouteHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*httpRouteHandler)(nil)

func NewHTTPRouteHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &httpRouteHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *httpRouteHandler) Name() string {
	return "KubernetesHTTPRouteHandler"
}

func (h *httpRouteHandler) IsRequired(deployCtx *dataplane.EndpointContext) bool {
	// HTTPRoutes are required for Web Applications
	return deployCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication
}

func (h *httpRouteHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	name := makeHTTPRouteName(deployCtx)
	out := &gatewayv1.HTTPRoute{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *httpRouteHandler) Create(ctx context.Context, deployCtx *dataplane.EndpointContext) error {
	httpRoute := makeHTTPRoute(deployCtx)
	return h.kubernetesClient.Create(ctx, httpRoute)
}

func (h *httpRouteHandler) Update(ctx context.Context, deployCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRoute, ok := currentState.(*gatewayv1.HTTPRoute)
	if !ok {
		return errors.New("failed to cast current state to HTTPRoute")
	}

	newHTTPRoute := makeHTTPRoute(deployCtx)

	if h.shouldUpdate(currentHTTPRoute, newHTTPRoute) {
		newHTTPRoute.ResourceVersion = currentHTTPRoute.ResourceVersion
		return h.kubernetesClient.Update(ctx, newHTTPRoute)
	}

	return nil
}

func (h *httpRouteHandler) Delete(ctx context.Context, deployCtx *dataplane.EndpointContext) error {
	httpRoute := makeHTTPRoute(deployCtx)
	err := h.kubernetesClient.Delete(ctx, httpRoute)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *httpRouteHandler) shouldUpdate(current, new *gatewayv1.HTTPRoute) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	return !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty())
}

func makeHTTPRoute(endpointCtx *dataplane.EndpointContext) *gatewayv1.HTTPRoute {
	return &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(endpointCtx),
			Namespace: makeNamespaceName(endpointCtx),
			Labels:    makeWorkloadLabels(endpointCtx),
		},
		Spec: makeHTTPRouteSpec(endpointCtx),
	}
}

func makeHTTPRouteName(endpointCtx *dataplane.EndpointContext) string {
	componentName := endpointCtx.Component.Name
	endpointName := endpointCtx.Endpoint.Name
	return dpkubernetes.GenerateK8sName(componentName, endpointName)
}

func makeHTTPRouteSpec(endpointCtx *dataplane.EndpointContext) gatewayv1.HTTPRouteSpec {
	pathType := gatewayv1.PathMatchPathPrefix
	hostname := gatewayv1.Hostname(endpointCtx.Component.Name + "-" + endpointCtx.Environment.Name + ".choreo.local")
	port := gatewayv1.PortNumber(endpointCtx.Endpoint.Spec.Service.Port)
	return gatewayv1.HTTPRouteSpec{
		CommonRouteSpec: gatewayv1.CommonRouteSpec{
			ParentRefs: []gatewayv1.ParentReference{
				{
					Name:      gatewayv1.ObjectName(gatewayExternal),
					Namespace: (*gatewayv1.Namespace)(ptr.String("choreo-system-dp")), // Change NS based on where envoy gateway is deployed
				},
			},
		},
		Hostnames: []gatewayv1.Hostname{hostname},
		Rules: []gatewayv1.HTTPRouteRule{
			{
				Matches: []gatewayv1.HTTPRouteMatch{
					{
						Path: &gatewayv1.HTTPPathMatch{
							Type:  &pathType,
							Value: ptr.String(endpointCtx.Endpoint.Spec.Service.BasePath),
						},
					},
				},
				BackendRefs: []gatewayv1.HTTPBackendRef{
					{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: gatewayv1.ObjectName(makeServiceName(endpointCtx)),
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
}

// NamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func makeNamespaceName(endpointCtx *dataplane.EndpointContext) string {
	organizationName := controller.GetOrganizationName(endpointCtx.Project)
	projectName := controller.GetName(endpointCtx.Project)
	environmentName := controller.GetName(endpointCtx.Environment)
	return dpkubernetes.GenerateK8sName("dp", organizationName, projectName, environmentName)
}

func makeServiceName(deployCtx *dataplane.EndpointContext) string {
	componentName := deployCtx.Component.Name
	deploymentTrackName := deployCtx.DeploymentTrack.Name
	return dpkubernetes.GenerateK8sName(componentName, deploymentTrackName)
}
