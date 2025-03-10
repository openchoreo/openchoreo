package argo

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations"
	"github.com/choreo-idp/choreo/internal/controller/build/integrations/kubernetes"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	argoproj "github.com/choreo-idp/choreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	"github.com/choreo-idp/choreo/internal/labels"
)

type workflowHandler struct {
	kubernetesClient client.Client
}

var _ integrations.ResourceHandler[integrations.BuildContext] = (*workflowHandler)(nil)

func NewWorkflowHandler(kubernetesClient client.Client) integrations.ResourceHandler[integrations.BuildContext] {
	return &workflowHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *workflowHandler) KindName() string {
	return "ArgoWorkflow"
}

func (h *workflowHandler) Name(ctx context.Context, builtCtx *integrations.BuildContext) string {
	return MakeWorkflowName(builtCtx)
}

func (h *workflowHandler) Get(ctx context.Context, builtCtx *integrations.BuildContext) (interface{}, error) {
	name := MakeWorkflowName(builtCtx)
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

// MakeWorkflowName generates the workflow name using the build name.
// WorkflowName is limited to 63 characters.
func MakeWorkflowName(buildCtx *integrations.BuildContext) string {
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

// ConstructImageNameWithTag creates the image name with the tag.
// This doesn't include git revision. It is added from the workflow.
func ConstructImageNameWithTag(build *choreov1.Build) string {
	orgName := build.ObjectMeta.Labels[labels.LabelKeyOrganizationName]
	projName := build.ObjectMeta.Labels[labels.LabelKeyProjectName]
	componentName := build.ObjectMeta.Labels[labels.LabelKeyComponentName]
	dtName := build.ObjectMeta.Labels[labels.LabelKeyDeploymentTrackName]

	// To prevent excessively long image names, we limit them to 128 characters for the name and 128 characters for the tag.
	imageName := dpkubernetes.GenerateK8sNameWithLengthLimit(128, orgName, projName, componentName)
	// The maximum recommended tag length is 128 characters, with 8 characters reserved for the commit SHA.
	return fmt.Sprintf(
		"%s:%s",
		imageName,
		dpkubernetes.GenerateK8sNameWithLengthLimit(119, dtName),
	)
}
