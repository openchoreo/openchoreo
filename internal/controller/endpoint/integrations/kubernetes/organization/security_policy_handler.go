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

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/endpoint/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/dataplane"
)

type SecurityPolicyHandler struct {
	client client.Client
}

var _ dataplane.ResourceHandler[dataplane.EndpointContext] = (*SecurityPolicyHandler)(nil)

func (h *SecurityPolicyHandler) Name() string {
	return "SecurityPolicy"
}

func (h *SecurityPolicyHandler) IsRequired(ctx *dataplane.EndpointContext) bool {
	if ctx.Endpoint.Spec.APISettings == nil || ctx.Endpoint.Spec.APISettings.SecuritySchemes == nil {
		return false
	}
	secSchemes := ctx.Endpoint.Spec.APISettings.SecuritySchemes
	for _, scheme := range secSchemes {
		return scheme == choreov1.Oauth
	}

	return false
}

func (h *SecurityPolicyHandler) GetCurrentState(ctx context.Context, epCtx *dataplane.EndpointContext) (interface{}, error) {
	namespace := kubernetes.MakeNamespaceName(epCtx)
	name := kubernetes.MakeHTTPRouteName(epCtx, kubernetes.GatewayExternal)
	out := &egv1a1.SecurityPolicy{}
	err := h.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *SecurityPolicyHandler) Create(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	securityPolicy := kubernetes.MakeSecurityPolicy(epCtx, kubernetes.GatewayInternal)
	return h.client.Create(ctx, securityPolicy)
}

func (h *SecurityPolicyHandler) Update(ctx context.Context, epCtx *dataplane.EndpointContext, currentState interface{}) error {
	current, ok := currentState.(*egv1a1.SecurityPolicy)
	if !ok {
		return errors.New("failed to cast current state to SecurityPolicy")
	}
	new := kubernetes.MakeSecurityPolicy(epCtx, kubernetes.GatewayInternal)
	if shouldUpdate(current, new) {
		new.ResourceVersion = current.ResourceVersion
		return h.client.Update(ctx, new)
	}
	return nil
}

func NewSecurityPolicyHandler(client client.Client) dataplane.ResourceHandler[dataplane.EndpointContext] {
	return &SecurityPolicyHandler{
		client: client,
	}
}

func (h *SecurityPolicyHandler) Delete(ctx context.Context, epCtx *dataplane.EndpointContext) error {
	return nil
}

func shouldUpdate(current, new *egv1a1.SecurityPolicy) bool {
	// Compare the labels
	if !cmp.Equal(kubernetes.ExtractManagedLabels(current.Labels), kubernetes.ExtractManagedLabels(new.Labels)) {
		return true
	}

	return !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty())
}
