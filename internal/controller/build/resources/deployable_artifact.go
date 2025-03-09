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

type deployableArtifactHandler struct {
	kubernetesClient client.Client
}

var _ common.ResourceHandler[common.BuildContext] = (*deployableArtifactHandler)(nil)

func NewDeployableArtifactHandler(kubernetesClient client.Client) common.ResourceHandler[common.BuildContext] {
	return &deployableArtifactHandler{
		kubernetesClient: kubernetesClient,
	}
}

func (h *deployableArtifactHandler) KindName() string {
	return "DeployableArtifact"
}

func (h *deployableArtifactHandler) Name(ctx context.Context, builtCtx *common.BuildContext) string {
	return makeDeployableArtifactName(builtCtx.Build)
}

func (h *deployableArtifactHandler) Get(ctx context.Context, builtCtx *common.BuildContext) (interface{}, error) {
	name := h.Name(ctx, builtCtx)
	deployableArtifact := &choreov1.DeployableArtifact{}
	err := h.kubernetesClient.Get(ctx, client.ObjectKey{Name: name}, deployableArtifact)
	if apierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return deployableArtifact, nil
}

func (h *deployableArtifactHandler) Create(ctx context.Context, builtCtx *common.BuildContext) error {
	deployableArtifact := makeDeployabeArtifact(builtCtx.Build)
	addComponentSpecificConfigs(builtCtx, deployableArtifact)
	return h.kubernetesClient.Create(ctx, deployableArtifact)
}

func (h *deployableArtifactHandler) Update(ctx context.Context, builtCtx *common.BuildContext, currentState interface{}) error {
	return nil
}

func makeDeployableArtifactName(build *choreov1.Build) string {
	return build.Name
}

func addComponentSpecificConfigs(buildCtx *common.BuildContext, deployableArtifact *choreov1.DeployableArtifact) {
	componentType := buildCtx.Component.Spec.Type
	if componentType == choreov1.ComponentTypeService {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: *buildCtx.Endpoints,
		}
	} else if componentType == choreov1.ComponentTypeScheduledTask {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			Application: &choreov1.Application{
				Task: &choreov1.TaskConfig{
					Disabled: false,
					Schedule: &choreov1.TaskSchedule{
						Cron:     "*/5 * * * *",
						Timezone: "Asia/Colombo",
					},
				},
			},
		}
	} else if componentType == choreov1.ComponentTypeWebApplication {
		deployableArtifact.Spec.Configuration = &choreov1.Configuration{
			EndpointTemplates: []choreov1.EndpointTemplate{
				{
					// TODO: This should come from the component descriptor in source code.
					ObjectMeta: metav1.ObjectMeta{
						Name: "webapp",
					},
					Spec: choreov1.EndpointSpec{
						Type: "HTTP",
						Service: choreov1.EndpointServiceSpec{
							BasePath: "/",
							Port:     80,
						},
					},
				},
			},
		}
	}
}

func makeDeployabeArtifact(build *choreov1.Build) *choreov1.DeployableArtifact {
	return &choreov1.DeployableArtifact{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DeployableArtifact",
			APIVersion: "core.choreo.dev/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeDeployableArtifactName(build),
			Namespace: build.Namespace,
			Annotations: map[string]string{
				"core.choreo.dev/display-name": makeDeployableArtifactName(build),
				"core.choreo.dev/description":  "Deployable Artifact was created by the build.",
			},
			Labels: map[string]string{
				labels.LabelKeyOrganizationName:    controller.GetOrganizationName(build),
				labels.LabelKeyProjectName:         controller.GetProjectName(build),
				labels.LabelKeyComponentName:       controller.GetComponentName(build),
				labels.LabelKeyDeploymentTrackName: controller.GetDeploymentTrackName(build),
				labels.LabelKeyName:                makeDeployableArtifactName(build),
				dpkubernetes.LabelKeyCreatedBy:     dpkubernetes.LabelBuildControllerCreated,
			},
		},
		Spec: choreov1.DeployableArtifactSpec{
			TargetArtifact: choreov1.TargetArtifact{
				FromBuildRef: &choreov1.FromBuildRef{
					Name: build.Name,
				},
			},
		},
	}
}
