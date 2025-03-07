package kubernetes

import (
	"github.com/choreo-idp/choreo/internal/controller"
	dpkubernetes "github.com/choreo-idp/choreo/internal/dataplane/kubernetes"
	"github.com/choreo-idp/choreo/internal/labels"
)

func makeNamespaceLabels(buildCtx *BuildContext) map[string]string {
	return map[string]string{
		dpkubernetes.LabelKeyOrganizationName: controller.GetOrganizationName(buildCtx.Build),
		dpkubernetes.LabelKeyProjectName:      controller.GetName(buildCtx.Build),
		dpkubernetes.LabelKeyCreatedBy:        dpkubernetes.LabelBuildControllerCreated,
		dpkubernetes.LabelKeyBelongTo:         buildCtx.Build.Labels[labels.LabelKeyOrganizationName],
	}
}
