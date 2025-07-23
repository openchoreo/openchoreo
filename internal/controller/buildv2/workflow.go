package buildv2

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/openchoreo/openchoreo/internal/labels"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	dpkubernetes "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes"
	argoproj "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
)

const (
	MaxWorkflowNameLength      = 63
	MaxImageNameLength         = 63
	MaxImageTagLength          = 128
	DefaultDTName              = "default"
	WorkflowServiceAccountName = "workflow-sa"
	WorkflowRoleName           = "workflow-role"
	WorkflowRoleBindingName    = "workflow-role-binding"
)

// makeArgoWorkflow creates an Argo Workflow from a BuildV2 resource
func makeArgoWorkflow(build *openchoreov1alpha1.BuildV2) *argoproj.Workflow {
	workflow := &argoproj.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      makeWorkflowName(build),
			Namespace: makeNamespaceName(build),
			Labels:    makeWorkflowLabels(build),
		},
		Spec: makeWorkflowSpec(build),
	}
	return workflow
}

// makeWorkflowSpec creates the workflow specification from a BuildV2 resource
func makeWorkflowSpec(build *openchoreov1alpha1.BuildV2) argoproj.WorkflowSpec {
	parameters := buildWorkflowParameters(build)

	return argoproj.WorkflowSpec{
		PodMetadata: &argoproj.Metadata{
			Labels: makeWorkflowLabels(build),
		},
		ServiceAccountName: WorkflowServiceAccountName,
		WorkflowTemplateRef: &argoproj.WorkflowTemplateRef{
			Name:         build.Spec.TemplateRef.Name,
			ClusterScope: true,
		},
		Arguments: argoproj.Arguments{
			Parameters: parameters,
		},
	}
}

// buildWorkflowParameters constructs the parameters for the workflow
func buildWorkflowParameters(build *openchoreov1alpha1.BuildV2) []argoproj.Parameter {

	parameters := []argoproj.Parameter{
		createParameter("git-repo", build.Spec.Repository.URL),
		createParameter("app-path", build.Spec.Repository.AppPath),
		createParameter("image-name", makeImageName(build)),
		createParameter("image-tag", makeImageTag(build)),
	}

	commit := build.Spec.Repository.Revision.Commit
	branch := build.Spec.Repository.Revision.Branch

	if commit != "" {
		branch = "" // ignore branch when commit is provided
	} else {
		if branch == "" {
			branch = "main"
		}
		commit = "" // ensure commit is empty when using branch
	}

	parameters = append(
		parameters,
		createParameter("commit", commit),
		createParameter("branch", branch),
	)

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

// makeImageName creates the image name following the pattern: project_name-component_name
func makeImageName(build *openchoreov1alpha1.BuildV2) string {
	projectName := normalizeForImageName(build.Spec.Owner.ProjectName)
	componentName := normalizeForImageName(build.Spec.Owner.ComponentName)

	imageName := fmt.Sprintf("%s-%s", projectName, componentName)

	// Ensure image name doesn't exceed maximum length
	if len(imageName) > MaxImageNameLength {
		imageName = imageName[:MaxImageNameLength]
		// Remove any trailing hyphens
		imageName = strings.TrimSuffix(imageName, "-")
	}

	return imageName
}

// makeImageTag creates the image tag
func makeImageTag(build *openchoreov1alpha1.BuildV2) string {
	tag := DefaultDTName
	return tag
}

// normalizeForImageName normalizes a string for use in image names
// Docker image names must be lowercase and can contain only alphanumeric characters, hyphens, and underscores
func normalizeForImageName(s string) string {
	// Convert to lowercase
	normalized := strings.ToLower(s)

	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-z0-9\-_]`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Remove consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	normalized = reg.ReplaceAllString(normalized, "-")

	// Remove leading and trailing hyphens
	normalized = strings.Trim(normalized, "-")

	return normalized
}

// makeWorkflowName generates a valid workflow name with length constraints
func makeWorkflowName(build *openchoreov1alpha1.BuildV2) string {
	return dpkubernetes.GenerateK8sNameWithLengthLimit(MaxWorkflowNameLength, build.Name)
}

// makeNamespaceName generates the namespace name for the workflow based on organization
func makeNamespaceName(build *openchoreov1alpha1.BuildV2) string {
	orgName := normalizeForK8s(build.Namespace)
	return fmt.Sprintf("openchoreo-ci-%s", orgName)
}

// makeWorkflowLabels creates labels for the workflow
func makeWorkflowLabels(build *openchoreov1alpha1.BuildV2) map[string]string {
	labels := map[string]string{
		labels.LabelKeyOrganizationName: normalizeForK8s(build.Namespace),
		labels.LabelKeyProjectName:      normalizeForK8s(build.Spec.Owner.ProjectName),
		labels.LabelKeyComponentName:    normalizeForK8s(build.Spec.Owner.ComponentName),
		labels.LabelKeyBuildName:        build.Name,
		labels.LabelKeyUUID:             string(build.UID),
		labels.LabelKeyTarget:           labels.LabelValueBuildTarget,
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

// makeNamespace creates a namespace for the build
func makeNamespace(build *openchoreov1alpha1.BuildV2) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   makeNamespaceName(build),
			Labels: makeWorkflowLabels(build),
		},
	}
}

// makeServiceAccount creates a service account for the workflow
func makeServiceAccount(build *openchoreov1alpha1.BuildV2) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowServiceAccountName,
			Namespace: makeNamespaceName(build),
			Labels:    makeWorkflowLabels(build),
		},
	}
}

// makeRole creates a role for the workflow
func makeRole(build *openchoreov1alpha1.BuildV2) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowRoleName,
			Namespace: makeNamespaceName(build),
			Labels:    makeWorkflowLabels(build),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"argoproj.io"},
				Resources: []string{"workflowtaskresults"},
				Verbs:     []string{"create", "get", "list", "watch", "update", "patch"},
			},
		},
	}
}

// makeRoleBinding creates a role binding for the workflow
func makeRoleBinding(build *openchoreov1alpha1.BuildV2) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkflowRoleBindingName,
			Namespace: makeNamespaceName(build),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      WorkflowServiceAccountName,
				Namespace: makeNamespaceName(build),
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     WorkflowRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
