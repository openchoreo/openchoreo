package resources

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/controller"
	"github.com/choreo-idp/choreo/internal/controller/build/common"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/labels"
)

type deploymentHandler struct {
	kubernetesClient client.Client
}

var _ common.ResourceHandler[common.BuildContext] = (*deploymentHandler)(nil)

func NewDeploymentHandler(kubernetesClient client.Client) common.ResourceHandler[common.BuildContext] {
	return &deploymentHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *deploymentHandler) KindName() string {
	return "DeployableArtifact"
}

func (h *deploymentHandler) Name(ctx context.Context, builtCtx *common.BuildContext) string {
	return makeDeployableArtifactName(builtCtx.Build)
}

func (h *deploymentHandler) Get(ctx context.Context, builtCtx *common.BuildContext) (interface{}, error) {
	name := h.Name(ctx, builtCtx)
	deployment := &choreov1.Deployment{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, deployment)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return deployment, nil
}

func (h *deploymentHandler) Create(ctx context.Context, builtCtx *common.BuildContext) error {
	deployableArtifact := MakeDeployment(builtCtx)
	return h.kubernetesClient.Create(ctx, deployableArtifact)
}

func (h *deploymentHandler) Update(ctx context.Context, builtCtx *common.BuildContext, currentState interface{}) error {
	return nil
}

func MakeDeployment(buildCtx *common.BuildContext) *choreov1.Deployment {
	return &choreov1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "core.choreo.dev/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeDeploymentName(buildCtx.Build, buildCtx.InitialEnvironment.Name),
			Namespace: buildCtx.Build.Namespace,
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    controller.GetOrganizationName(buildCtx.Build),
				labels.LabelKeyProjectName:         controller.GetProjectName(buildCtx.Build),
				labels.LabelKeyComponentName:       controller.GetComponentName(buildCtx.Build),
				labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(buildCtx.Build),
				labels.LabelKeyEnvironmentName:     buildCtx.InitialEnvironment.Name,
				labels.LabelKeyName:                makeDeploymentLabelName(buildCtx.InitialEnvironment.Name),
				dpkubernetes.LabelKeyCreatedBy:     dpkubernetes.LabelBuildControllerCreated,
			},
		},
		Spec: choreov1.DeploymentSpec{
			DeploymentArtifactRef: buildCtx.Build.Name,
		},
	}
}

func makeDeploymentLabelName(environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(63, environmentName, "deployment")
}

func makeDeploymentName(build *choreov1.Build, environmentName string) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(
		dpkubernetes.MaxResourceNameLength,
		controller.GetOrganizationName(build),
		controller.GetProjectName(build),
		controller.GetComponentName(build),
		controller.GetDeploymentTrackName(build),
		environmentName,
	)
}
