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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/dataplane"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
)

type ciliumNetworkPolicyHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*ciliumNetworkPolicyHandler)(nil)

func NewCiliumNetworkPolicyHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &ciliumNetworkPolicyHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *ciliumNetworkPolicyHandler) Name() string {
	return "KubernetesCiliumNetworkPolicy"
}

func (h *ciliumNetworkPolicyHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	// CiliumNetworkPolicy is always required and the deletion of this should be handled by the project deletion
	// This will ensure the CiliumNetworkPolicy is lazily created during the first deployment for a namespace
	return true
}

func (h *ciliumNetworkPolicyHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	namespace := makeNamespaceName(deployCtx)
	name := makeCiliumNetworkPolicyName(deployCtx)
	out := &ciliumv2.CiliumNetworkPolicy{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *ciliumNetworkPolicyHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	cnp := makeCiliumNetworkPolicy(deployCtx)
	return h.kubernetesClient.Create(ctx, cnp)
}

func (h *ciliumNetworkPolicyHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentCNP, ok := currentState.(*ciliumv2.CiliumNetworkPolicy)
	if !ok {
		return errors.New("failed to cast current state to CiliumNetworkPolicy")
	}
	newCNP := makeCiliumNetworkPolicy(deployCtx)

	if h.shouldUpdate(currentCNP, newCNP) {
		newCNP.ResourceVersion = currentCNP.ResourceVersion
		return h.kubernetesClient.Update(ctx, newCNP)
	}

	return nil
}

func (h *ciliumNetworkPolicyHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	cnp := makeCiliumNetworkPolicy(deployCtx)
	err := h.kubernetesClient.Delete(ctx, cnp)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *ciliumNetworkPolicyHandler) shouldUpdate(current, new *ciliumv2.CiliumNetworkPolicy) bool {
	// Compare the labels
	if !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels)) {
		return true
	}

	if !cmp.Equal(current.Spec, new.Spec, cmpopts.EquateEmpty()) {
		return true
	}
	return false
}

func makeCiliumNetworkPolicyName(deployCtx *dataplane.DeploymentContext) string {
	return "default-policy"
}

func makeCiliumNetworkPolicy(deployCtx *dataplane.DeploymentContext) *ciliumv2.CiliumNetworkPolicy {
	return &ciliumv2.CiliumNetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "cilium.io/v2",
			Kind:       "CiliumNetworkPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeCiliumNetworkPolicyName(deployCtx),
			Namespace: makeNamespaceName(deployCtx),
			Labels:    makeNamespaceLabels(deployCtx),
		},
		Spec: makeRuleAllowCommunicationWithinNamespaceOnly(),
	}
}

func makeRuleAllowCommunicationWithinNamespaceOnly() *ciliumv2.Rule {
	allEndpoints := ciliumv2.EndpointSelector{}

	// Allow all pods in the namespace to communicate with each other
	// The EndpointSelector is empty which means it selects all endpoints (pods) in the namespace
	// The Egress and Ingress rules are defined to allow all pods in the namespace to communicate with each other
	return &ciliumv2.Rule{
		EndpointSelector: &allEndpoints,
		Egress: []ciliumv2.EgressRule{
			{
				ToEndpoints: []ciliumv2.EndpointSelector{allEndpoints},
			},
		},
		Ingress: []ciliumv2.IngressRule{
			{
				FromEndpoints: []ciliumv2.EndpointSelector{allEndpoints},
			},
		},
	}
}
