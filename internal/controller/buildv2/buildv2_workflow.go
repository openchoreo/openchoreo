package buildv2

import (
	"fmt"
	"strings"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	MaxWorkflowNameLength = 63
	DefaultNamespace      = "choreo-ci-default-org"
)

// makeArgoWorkflow creates an Argo Workflow from a BuildV2 resource
func makeArgoWorkflow(build *choreov1.BuildV2) *argoproj.Workflow {
	workflow := &argoproj.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeWorkflowName(build),
			Namespace: makeNamespaceName(build),
			Labels:    makeWorkflowLabels(build),
			Annotations: map[string]string{
				"buildv2.core.choreo.dev/build-name":     build.Name,
				"buildv2.core.choreo.dev/organization":   build.Spec.Owner.OrganizationName,
				"buildv2.core.choreo.dev/project":        build.Spec.Owner.ProjectName,
				"buildv2.core.choreo.dev/component":      build.Spec.Owner.ComponentName,
				"buildv2.core.choreo.dev/repository-url": build.Spec.Repository.URL,
			},
		},
		Spec: makeWorkflowSpec(build),
	}
	return workflow
}

// makeWorkflowSpec creates the workflow specification from a BuildV2 resource
func makeWorkflowSpec(build *choreov1.BuildV2) argoproj.WorkflowSpec {
	parameters := buildWorkflowParameters(build)

	return argoproj.WorkflowSpec{
		WorkflowTemplateRef: &argoproj.WorkflowTemplateRef{
			Name: build.Spec.TemplateRef.Name,
		},
		Arguments: argoproj.Arguments{
			Parameters: parameters,
		},
	}
}

// buildWorkflowParameters constructs the parameters for the workflow
func buildWorkflowParameters(build *choreov1.BuildV2) []argoproj.Parameter {
	var parameters []argoproj.Parameter

	// Add core parameters
	parameters = append(parameters,
		createParameter("url", build.Spec.Repository.URL),
		createParameter("appPath", build.Spec.Repository.AppPath),
	)

	// Add revision parameter (branch or commit)
	if build.Spec.Repository.Revision.Commit != "" {
		parameters = append(parameters, createParameter("commit", build.Spec.Repository.Revision.Commit))
	} else if build.Spec.Repository.Revision.Branch != "" {
		parameters = append(parameters, createParameter("branch", build.Spec.Repository.Revision.Branch))
	}

	// Add template-specific parameters
	for _, param := range build.Spec.TemplateRef.Parameters {
		parameters = append(parameters, createParameter(param.Name, param.Value))
	}

	return parameters
}

// createParameter creates a workflow parameter with proper type conversion
func createParameter(name, value string) argoproj.Parameter {
	paramValue := argoproj.AnyString(value)
	return argoproj.Parameter{
		Name:  name,
		Value: &paramValue,
	}
}

// makeWorkflowName generates a valid workflow name with length constraints
func makeWorkflowName(build *choreov1.BuildV2) string {
	baseName := fmt.Sprintf("%s-build", build.Name)
	return dpkubernetes.GenerateK8sNameWithLengthLimit(MaxWorkflowNameLength, baseName)
}

// makeNamespaceName generates the namespace name for the workflow
func makeNamespaceName(build *choreov1.BuildV2) string {
	// Use organization-specific namespace if available
	if build.Spec.Owner.OrganizationName != "" {
		orgNamespace := fmt.Sprintf("choreo-ci-%s", normalizeForK8s(build.Spec.Owner.OrganizationName))
		return dpkubernetes.GenerateK8sNameWithLengthLimit(MaxWorkflowNameLength, orgNamespace)
	}
	return DefaultNamespace
}

// makeWorkflowLabels creates labels for the workflow
func makeWorkflowLabels(build *choreov1.BuildV2) map[string]string {
	labels := map[string]string{
		dpkubernetes.LabelKeyManagedBy:     dpkubernetes.LabelBuildControllerCreated,
		"core.openchoreo.dev/build-name":   build.Name,
		"core.openchoreo.dev/organization": normalizeForK8s(build.Spec.Owner.OrganizationName),
		"core.openchoreo.dev/project":      normalizeForK8s(build.Spec.Owner.ProjectName),
		"core.openchoreo.dev/component":    normalizeForK8s(build.Spec.Owner.ComponentName),
	}
	return labels
}

// normalizeForK8s normalizes a string to be valid for Kubernetes labels/names
func normalizeForK8s(s string) string {
	// Replace invalid characters with hyphens
	normalized := strings.ReplaceAll(s, "_", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")
	normalized = strings.ToLower(normalized)

	// Ensure it starts and ends with alphanumeric characters
	normalized = strings.Trim(normalized, "-")

	// Limit length for labels (63 characters max)
	if len(normalized) > 63 {
		normalized = normalized[:63]
		normalized = strings.TrimSuffix(normalized, "-")
	}

	return normalized
}
