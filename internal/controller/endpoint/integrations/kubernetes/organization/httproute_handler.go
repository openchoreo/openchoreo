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

package organization

import (
	"context"
	"errors"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/dataplane"
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
	if epCtx.Component.Spec.Type == choreov1.ComponentTypeWebApplication {
		return false
	}
	return epCtx.Endpoint.Spec.NetworkVisibilities.Organization != nil &&
		epCtx.Endpoint.Spec.NetworkVisibilities.Organization.Enable
}

func (h *httpRouteHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := kubernetes.MakeNamespaceName(epCtx)
	name := kubernetes.MakeHTTPRouteName(epCtx, kubernetes.GatewayExternal)
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
	httpRoute := kubernetes.MakeHTTPRoute(epCtx, kubernetes.GatewayExternal)
	return h.client.Create(ctx, httpRoute)
}

func (h *httpRouteHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	currentHTTPRoute, ok := currentState.(*gwapiv1.HTTPRoute)
	if !ok {
		return errors.New("failed to cast current state to HTTPRoute")
	}

	newHTTPRoute := kubernetes.MakeHTTPRoute(epCtx, kubernetes.GatewayExternal)

	if h.shouldUpdate(currentHTTPRoute, newHTTPRoute) {
		newHTTPRoute.ResourceVersion = currentHTTPRoute.ResourceVersion
		return h.client.Update(ctx, newHTTPRoute)
	}

	return nil
}

func (h *httpRouteHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	httpRoute := kubernetes.MakeHTTPRoute(epCtx, kubernetes.GatewayExternal)
	err := h.client.Delete(ctx, httpRoute)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *httpRouteHandler) shouldUpdate(current, new *gwapiv1.HTTPRoute) bool {
	// Compare the labels
	if !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels)) {
		return true
	}

	return !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty())
}
