/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package kubernetes

import (
	"context"
	"errors"
	"path"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/ptr"
)

// Gateway Types
const (
	gatewayExternal = "gateway-external"
	gatewayInternal = "gateway-internal"
)

type httpRouteHandler struct {
	client client.Client
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*httpRouteHandler)(nil)

func NewHTTPRouteHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &httpRouteHandler{
		client: kubernetesClient,
	}
}

func (h *httpRouteHandler) Name() string {
	return "KubernetesHTTPRouteHandler"
}

func (h *httpRouteHandler) IsRequired(epCtx *dataplane.EndpointContext) bool {
	// HTTPRoutes are required for Web Applications
	return epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication ||
		epCtx.Component.Spec.Type == choreov1.ComponentTypeService
}

func (h *httpRouteHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	name := makeHTTPRouteName(epCtx)
	out := &gwapiv1.HTTPRoute{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *httpRouteHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRoute := makeHTTPRoute(epCtx)
	return h.client.Create(ctx, httpRoute)
}

func (h *httpRouteHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRoute, ok := currentState.(*gwapiv1.HTTPRoute)
	if !ok {
		return errors.New("failed to cast current state to HTTPRoute")
	}

	newHTTPRoute := makeHTTPRoute(epCtx)

	if h.shouldUpdate(currentHTTPRoute, newHTTPRoute) {
		newHTTPRoute.ResourceVersion = currentHTTPRoute.ResourceVersion
		return h.client.Update(ctx, newHTTPRoute)
	}

	return nil
}

func (h *httpRouteHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRoute := makeHTTPRoute(epCtx)
	err := h.client.Delete(ctx, httpRoute)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *httpRouteHandler) shouldUpdate(current, new *gwapiv1.HTTPRoute) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	return !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty())
}

func makeHTTPRoute(epCtx *dataplane.EndpointContext) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(epCtx),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx),
		},
		Spec: makeHTTPRouteSpec(epCtx),
	}
}

func makeHTTPRouteName(epCtx *dataplane.EndpointContext) string {
	componentName := epCtx.Component.Name
	endpointName := epCtx.Endpoint.Name
	return dpkubernetes.GenerateK8sName(componentName, endpointName)
}

func makeHTTPRouteSpec(epCtx *dataplane.EndpointContext) gwapiv1.HTTPRouteSpec {
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(epCtx.Component.Name, epCtx.Environment.Name, epCtx.Component.Spec.Type)
	port := gwapiv1.PortNumber(epCtx.Endpoint.Spec.Service.Port)
	prefix := makePathPrefix(epCtx.Project.Name, epCtx.Component.Name, epCtx.Component.Spec.Type)
	basePath := epCtx.Endpoint.Spec.Service.BasePath
	endpointPath := basePath
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeService {
		// Prefix basepath with project and component names TODO: add org if necessary
		endpointPath = path.Clean(path.Join(prefix, basePath))
	}
	return gwapiv1.HTTPRouteSpec{
		CommonRouteSpec: gwapiv1.CommonRouteSpec{
			ParentRefs: []gwapiv1.ParentReference{
				{
					Name:      gwapiv1.ObjectName(gatewayExternal),
					Namespace: (*gwapiv1.Namespace)(ptr.String("choreo-system")), // Change NS based on where envoy gateway is deployed
				},
			},
		},
		Hostnames: []gwapiv1.Hostname{hostname},
		Rules: []gwapiv1.HTTPRouteRule{
			{
				Matches: []gwapiv1.HTTPRouteMatch{
					{
						Path: &gwapiv1.HTTPPathMatch{
							Type:  &pathType,
							Value: ptr.String(endpointPath),
						},
					},
				},
				Filters: []gwapiv1.HTTPRouteFilter{
					{
						Type: gwapiv1.HTTPRouteFilterURLRewrite,
						URLRewrite: &gwapiv1.HTTPURLRewriteFilter{
							Path: &gwapiv1.HTTPPathModifier{
								Type:               gwapiv1.PrefixMatchHTTPPathModifier,
								ReplacePrefixMatch: ptr.String(basePath),
							},
						},
					},
				},
				BackendRefs: []gwapiv1.HTTPBackendRef{
					{
						BackendRef: gwapiv1.BackendRef{
							BackendObjectReference: gwapiv1.BackendObjectReference{
								Name: gwapiv1.ObjectName(makeServiceName(epCtx)),
								Port: &port,
							},
						},
					},
				},
			},
		},
	}
}
