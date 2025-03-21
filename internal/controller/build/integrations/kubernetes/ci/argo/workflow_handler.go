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

package argo

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	"github.com/choreo-idp/choreo/internal/dataplane"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	argoproj "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

type workflowHandler struct {
	kubernetesClient client.Client
}

var _ dataplane.ResourceHandler[integrations.BuildContext] = (*workflowHandler)(nil)

func NewWorkflowHandler(kubernetesClient client.Client) dataplane.ResourceHandler[integrations.BuildContext] {
	return &workflowHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *workflowHandler) Name() string {
	return "ArgoWorkflow"
}

func (h *workflowHandler) GetCurrentState(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
	name := makeWorkflowName(builtCtx)
	workflow := argoproj.Workflow{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name, Namespace: kubernetes.MakeNamespaceName(builtCtx)}, &workflow)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return workflow, nil
}

func (h *workflowHandler) Create(ctx context.Context, builtCtx *integrations.BuildContext) error {
	workflow := makeArgoWorkflow(builtCtx)
	return h.kubernetesClient.Create(ctx, workflow)
}

func (h *workflowHandler) Update(ctx context.Context, builtCtx *integrations.BuildContext, currentState interface{}) error {
	return nil
}

func (h *workflowHandler) Delete(ctx context.Context, builtCtx *integrations.BuildContext) error {
	workflow := makeArgoWorkflow(builtCtx)
	err := h.kubernetesClient.Delete(ctx, workflow)
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (h *workflowHandler) IsRequired(builtCtx *integrations.BuildContext) bool {
	return true
}

// makeWorkflowName generates the workflow name using the build name.
// WorkflowName is limited to 63 characters.
func makeWorkflowName(buildCtx *integrations.BuildContext) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(63, buildCtx.Build.ObjectMeta.Name)
}

func GetStepPhase(phase argoproj.NodePhase) integrations.StepPhase {
	switch phase {
	case argoproj.NodeRunning, argoproj.NodePending:
		return integrations.Running
	case argoproj.NodeFailed, argoproj.NodeError, argoproj.NodeSkipped:
		return integrations.Failed
	default:
		return integrations.Succeeded
	}
}

func GetStepByTemplateName(nodes argoproj.Nodes, step integrations.BuildWorkflowStep) (*argoproj.NodeStatus, bool) {
	for _, node := range nodes {
		if node.TemplateName == string(step) {
			return &node, true
		}
	}
	return nil, false
}

func GetImageNameFromWorkflow(output argoproj.Outputs) string {
	for _, param := range output.Parameters {
		if param.Name == "image" {
			return *param.Value
		}
	}
	return ""
}
