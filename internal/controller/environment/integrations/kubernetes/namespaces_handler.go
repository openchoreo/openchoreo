/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
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
	// this should list the namespaces which has the following labels:
	//	environment-name: <environment_name>
	//	organization-name: <organization_name>
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
	// this should delete the namespaces which has the following labels:
	//	environment-name: <environment_name>
	//	organization-name: <organization_name>
	namespaceList := &corev1.NamespaceList{}
	labelSelector := client.MatchingLabels{
		k8s.LabelKeyEnvironmentName:  envCtx.Environment.Name,
		k8s.LabelKeyOrganizationName: envCtx.Environment.Namespace,
	}

	if err := h.kubernetesClient.List(ctx, namespaceList, labelSelector); err != nil {
		return fmt.Errorf("error listing namespaces: %w", err)
	}

	if len(namespaceList.Items) == 0 {
		return nil
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
