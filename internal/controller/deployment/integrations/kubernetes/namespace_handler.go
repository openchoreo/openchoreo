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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

type namespaceHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.DeploymentContext] = (*namespaceHandler)(nil)

func NewNamespaceHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.DeploymentContext] {
	return &namespaceHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *namespaceHandler) Name() string {
	return "KubernetesNamespace"
}

func (h *namespaceHandler) IsRequired(deployCtx *dataplane.DeploymentContext) bool {
	// Namespace is always required and the deletion of a namespace should be handled by the project deletion
	// This will ensure the namespace is lazily created during the first deployment
	return true
}

func (h *namespaceHandler) GetCurrentState(ctx context.Context, deployCtx *dataplane.DeploymentContext) (interface{}, error) {
	name := makeNamespaceName(deployCtx)
	out := &corev1.Namespace{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, out)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *namespaceHandler) Create(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	namespace := makeNamespace(deployCtx)
	return h.kubernetesClient.Create(ctx, namespace)
}

func (h *namespaceHandler) Update(ctx context.Context, deployCtx *dataplane.DeploymentContext, currentState interface{}) error {
	currentNamespace, ok := currentState.(*corev1.Namespace)
	if !ok {
		return errors.New("failed to cast the current state to a Namespace")
	}
	newNamespace := makeNamespace(deployCtx)

	if h.shouldUpdate(currentNamespace, newNamespace) {
		return h.kubernetesClient.Update(ctx, newNamespace)
	}

	return nil
}

func (h *namespaceHandler) Delete(ctx context.Context, deployCtx *dataplane.DeploymentContext) error {
	// Namespaces should not be deleted by the deployment controller as they are cleaned up by the project deletion
	return nil
}

func (h *namespaceHandler) shouldUpdate(current, new *corev1.Namespace) bool {
	// Compare only the labels
	return !cmp.Equal(extractManagedLabels(current.Labels), extractManagedLabels(new.Labels))
}

// NamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func makeNamespaceName(deployCtx *dataplane.DeploymentContext) string {
	organizationName := controller.GetOrganizationName(deployCtx.Project)
	projectName := controller.GetName(deployCtx.Project)
	environmentName := controller.GetName(deployCtx.Environment)
	// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
	return dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
		"dp", organizationName, projectName, environmentName)
}

func makeNamespace(deployCtx *dataplane.DeploymentContext) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   makeNamespaceName(deployCtx),
			Labels: makeNamespaceLabels(deployCtx),
		},
	}
}
