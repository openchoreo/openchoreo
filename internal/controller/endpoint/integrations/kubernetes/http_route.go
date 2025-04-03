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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller/endpoint/integrations/kubernetes/visibility"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	"github.com/openchoreo/openchoreo/internal/ptr"
)

type httpRouteHandler struct {
	client     client.Client
	visibility visibility.VisibilityStrategy
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*httpRouteHandler)(nil)

func NewHTTPRouteHandler(kubernetesClient client.Client, visibility visibility.VisibilityStrategy) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &httpRouteHandler{
		client:     kubernetesClient,
		visibility: visibility,
	}
}

func (h *httpRouteHandler) Name() string {
	return "KubernetesHTTPRouteHandler"
}

func (h *httpRouteHandler) IsRequired(epCtx *dataplane.EndpointContext) bool {
	return h.visibility.IsHTTPRouteRequired(epCtx)
}

func (h *httpRouteHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := makeNamespaceName(epCtx)
	name := makeHTTPRouteName(epCtx, h.visibility.GetGatewayType())
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
	httpRoute := MakeHTTPRoute(epCtx, h.visibility.GetGatewayType())
	return h.client.Create(ctx, httpRoute)
}

func (h *httpRouteHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRoute, ok := currentState.(*gwapiv1.HTTPRoute)
	if !ok {
		return errors.New("failed to cast current state to HTTPRoute")
	}

	newHTTPRoute := MakeHTTPRoute(epCtx, h.visibility.GetGatewayType())

	if h.shouldUpdate(currentHTTPRoute, newHTTPRoute) {
		newHTTPRoute.ResourceVersion = currentHTTPRoute.ResourceVersion
		return h.client.Update(ctx, newHTTPRoute)
	}

	return nil
}

func (h *httpRouteHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRoute := MakeHTTPRoute(epCtx, h.visibility.GetGatewayType())
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

func MakeHTTPRoute(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) *gwapiv1.HTTPRoute {
	return &gwapiv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeHTTPRouteName(epCtx, gwType),
			Namespace: makeNamespaceName(epCtx),
			Labels:    makeWorkloadLabels(epCtx),
		},
		Spec: makeHTTPRouteSpec(epCtx, gwType),
	}
}

func makeHTTPRouteSpec(epCtx *dataplane.EndpointContext, gwType visibility.GatewayType) gwapiv1.HTTPRouteSpec {
	updatedEp := visibility.OverrideAPISettings(epCtx, gwType)
	pathType := gwapiv1.PathMatchPathPrefix
	hostname := makeHostname(epCtx, gwType)
	port := gwapiv1.PortNumber(updatedEp.Spec.Service.Port)
	prefix := makePathPrefix(epCtx)
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
					Name:      gwapiv1.ObjectName(gwType),
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
