package argo

import (
	"context"
	"github.com/choreo-idp/choreo/internal/controller/build"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	argoproj "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type workflowHandler struct {
	kubernetesClient client.Client
}

var _ kubernetes.ResourceHandler[kubernetes.BuildContext] = (*workflowHandler)(nil)

func NewWorkflowHandler(kubernetesClient client.Client) kubernetes.ResourceHandler[kubernetes.BuildContext] {
	return &workflowHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *workflowHandler) KindName() string {
	return "ArgoWorkflow"
}

func (h *workflowHandler) Name(ctx context.Context, builtCtx *kubernetes.BuildContext) string {
	return makeWorkflowName(builtCtx)
}

func (h *workflowHandler) Get(ctx context.Context, builtCtx *kubernetes.BuildContext) (interface{}, error) {
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

func (h *workflowHandler) Create(ctx context.Context, builtCtx *kubernetes.BuildContext) error {
	workflow := makeArgoWorkflow(builtCtx)
	return h.kubernetesClient.Create(ctx, workflow)
}

// WorkflowName is the build name
func makeWorkflowName(builtCtx *kubernetes.BuildContext) string {
	return builtCtx.Build.Name
}

func GetStepPhase(phase argoproj.NodePhase) build.StepPhase {
	switch phase {
	case argoproj.NodeRunning, argoproj.NodePending:
		return build.Running
	case argoproj.NodeFailed, argoproj.NodeError, argoproj.NodeSkipped:
		return build.Failed
	default:
		return build.Succeeded
	}
}

func GetStepByTemplateName(nodes argoproj.Nodes, step build.BuildWorkflowStep) (*argoproj.NodeStatus, bool) {
	for _, node := range nodes {
		if node.TemplateName == string(step) {
			return &node, true
		}
	}
	return nil, false
}
