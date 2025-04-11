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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
)

type namespaceHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[dataplane.ProjectContext] = (*namespaceHandler)(nil)

func NewNamespaceHandler(kubernetesClient client.Client) dataplane.ResourceHandler[dataplane.ProjectContext] {
	return &namespaceHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *namespaceHandler) Name() string {
	return "KubernetesNamespace"
}

func (h *namespaceHandler) IsRequired(deployCtx *dataplane.ProjectContext) bool {
	// Namespace is always required and the deletion of a namespace should be handled by the project deletion
	// This will ensure the namespace is lazily created during the first deployment
	return true
}

func (h *namespaceHandler) GetCurrentState(ctx context.Context, projectCtx *dataplane.ProjectContext) (interface{}, error) {
	names := projectCtx.NamespaceNames
	atLeastOneFound := false
	var out *corev1.Namespace

	for _, name := range names {
		out = &corev1.Namespace{}
		err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, out)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		atLeastOneFound = true
	}

	if atLeastOneFound {
		return out, nil
	}
	// If no namespace is found, return nil
	return nil, nil
}

func (h *namespaceHandler) Create(ctx context.Context, deployCtx *dataplane.ProjectContext) error {
	return nil
}

func (h *namespaceHandler) Update(ctx context.Context, deployCtx *dataplane.ProjectContext, currentState interface{}) error {
	return nil
}

func (h *namespaceHandler) Delete(ctx context.Context, deployCtx *dataplane.ProjectContext) error {
	for _, name := range deployCtx.NamespaceNames {
		out := &corev1.Namespace{}
		err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, out)
		if apierrors.IsNotFound(err) {
			continue
		} else if err != nil {
			return err
		}

		err = h.kubernetesClient.Delete(ctx, out)
		if err != nil {
			return fmt.Errorf("error while deleting Namespace: %w", err)
		}
		return nil
	}
	return nil
}

// MakeNamespaceNames generates Kubernetes namespace names for each environment in the project
// NamespaceName has the format dp-<organization-name>-<project-name>-<environment-name>-<hash>
func MakeNamespaceNames(environmentNames []string, project choreov1.Project) []string {
	namespaceNames := make([]string, 0, len(environmentNames))

	organizationName := controller.GetOrganizationName(&project)
	projectName := controller.GetName(&project)
	for _, env := range environmentNames {
		environmentName := env
		// Limit the name to 63 characters to comply with the K8s name length limit for Namespaces
		namespaceName := dpkubernetes.GenerateK8sNameWithLengthLimit(dpkubernetes.MaxNamespaceNameLength,
			"dp", organizationName, projectName, environmentName)
		namespaceNames = append(namespaceNames, namespaceName)
	}

	return namespaceNames
}
