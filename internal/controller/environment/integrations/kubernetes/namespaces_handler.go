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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/dataplane"
	k8s "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

type namespacesHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.EnvironmentContext] = (*namespacesHandler)(nil)

func NewNamespacesHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.EnvironmentContext] {
	return &namespacesHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *namespacesHandler) Name() string {
	return "KubernetesNamespaces"
}

func (h *namespacesHandler) IsRequired(envCtx *dataplane.EnvironmentContext) bool {
	return true
}

func (h *namespacesHandler) GetCurrentState(ctx context.Context, envCtx *dataplane.EnvironmentContext) (interface{}, error) {
	// this should list the set of namespaces which has the label of the environment-name: <name>
	namespaceList := &corev1.NamespaceList{}
	labelSelector := client.MatchingLabels{
		k8s.LabelKeyEnvironmentName:  envCtx.Environment.Name,
		k8s.LabelKeyOrganizationName: envCtx.Environment.Namespace,
	}
	if err := h.kubernetesClient.List(ctx, namespaceList, labelSelector); err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error listing namespaces: %w", err)
	}
	if len(namespaceList.Items) > 0 {
		return namespaceList, nil
	}
	return nil, nil
}

func (h *namespacesHandler) Create(ctx context.Context, envCtx *dataplane.EnvironmentContext) error {
	return nil
}

func (h *namespacesHandler) Update(ctx context.Context, envCtx *dataplane.EnvironmentContext, currentState interface{}) error {
	return nil
}

func (h *namespacesHandler) Delete(ctx context.Context, envCtx *dataplane.EnvironmentContext) error {
	// this should delete the set of namespaces which has the label of the environment-name: <name>
	namespaceList := &corev1.NamespaceList{}
	labelSelector := client.MatchingLabels{
		k8s.LabelKeyEnvironmentName: envCtx.Environment.Name,
	}

	if err := h.kubernetesClient.List(ctx, namespaceList, labelSelector); err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("error listing namespaces: %w", err)
	}

	// Deleting each namespace
	for _, ns := range namespaceList.Items {
		if err := h.kubernetesClient.Delete(ctx, &ns); err != nil {
			if k8sapierrors.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("error deleting namespace %s: %w", ns.Name, err)
		}
	}
	return nil
}
