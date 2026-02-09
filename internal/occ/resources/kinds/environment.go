// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/occ/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// EnvironmentResource provides operations for Environment CRs.
type EnvironmentResource struct {
	*resources.BaseResource[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList]
}

// NewEnvironmentResource constructs an EnvironmentResource with CRDConfig and optionally sets namespace.
func NewEnvironmentResource(cfg constants.CRDConfig, namespace string) (*EnvironmentResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList]{
		resources.WithClient[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList](cli),
		resources.WithConfig[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList](cfg),
	}

	// Add namespace namespace if provided
	if namespace != "" {
		options = append(options, resources.WithNamespace[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList](namespace))
	}

	return &EnvironmentResource{
		BaseResource: resources.NewBaseResource[*openchoreov1alpha1.Environment, *openchoreov1alpha1.EnvironmentList](options...),
	}, nil
}

// WithNamespace sets the namespace for the environment resource (usually the namespace name)
func (e *EnvironmentResource) WithNamespace(namespace string) {
	e.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of an Environment with detailed information.
func (e *EnvironmentResource) GetStatus(env *openchoreov1alpha1.Environment) string {
	// Environment can have Ready or Configured conditions
	priorityConditions := []string{ConditionTypeReady, ConditionTypeConfigured}

	return resources.GetResourceStatus(
		env.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of an Environment.
func (e *EnvironmentResource) GetAge(env *openchoreov1alpha1.Environment) string {
	return resources.FormatAge(env.GetCreationTimestamp().Time)
}

// PrintTableItems formats environments into a table
func (e *EnvironmentResource) PrintTableItems(environments []resources.ResourceWrapper[*openchoreov1alpha1.Environment]) error {
	if len(environments) == 0 {
		// Provide a more descriptive message
		namespaceName := e.GetNamespace()

		message := "No environments found"

		if namespaceName != "" {
			message += " in namespace " + namespaceName
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(environments))

	for _, wrapper := range environments {
		env := wrapper.Resource
		// Format DataPlaneRef for display
		dataPlaneRefStr := ""
		if env.Spec.DataPlaneRef != nil {
			dataPlaneRefStr = fmt.Sprintf("%s/%s", env.Spec.DataPlaneRef.Kind, env.Spec.DataPlaneRef.Name)
		}
		rows = append(rows, []string{
			wrapper.LogicalName,
			resources.FormatValueOrPlaceholder(dataPlaneRefStr),
			resources.FormatBoolAsYesNo(env.Spec.IsProduction),
			resources.FormatValueOrPlaceholder(env.Spec.Gateway.DNSPrefix),
			resources.FormatAge(env.GetCreationTimestamp().Time),
			env.GetLabels()[constants.LabelNamespace],
		})
	}
	return resources.PrintTable(HeadersEnvironment, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (e *EnvironmentResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	environments, err := e.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(environments, filter.Name)
		if err != nil {
			return err
		}
		environments = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return e.PrintTableItems(environments)
	case resources.OutputFormatYAML:
		return e.BaseResource.PrintItems(environments, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateEnvironment creates a new Environment CR.
func (e *EnvironmentResource) CreateEnvironment(params api.CreateEnvironmentParams) error {
	// Generate a K8s-compliant name for the environment
	k8sName := resources.GenerateResourceName(params.Namespace, params.Name)

	// Convert DataPlaneRef string to object
	// If specified, default to DataPlane kind unless explicitly set
	var dataPlaneRef *openchoreov1alpha1.DataPlaneRef
	if params.DataPlaneRef != "" {
		dataPlaneRef = &openchoreov1alpha1.DataPlaneRef{
			Kind: openchoreov1alpha1.DataPlaneRefKindDataPlane,
			Name: params.DataPlaneRef,
		}
	}

	// Create the Environment resource
	environment := &openchoreov1alpha1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Namespace,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: resources.DefaultIfEmpty(params.DisplayName, params.Name),
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:      params.Name,
				constants.LabelNamespace: params.Namespace,
			},
		},
		Spec: openchoreov1alpha1.EnvironmentSpec{
			DataPlaneRef: dataPlaneRef,
			IsProduction: params.IsProduction,
			Gateway: openchoreov1alpha1.GatewayConfig{
				DNSPrefix: params.DNSPrefix,
			},
		},
	}

	// Create the environment using the base create method
	if err := e.Create(environment); err != nil {
		return fmt.Errorf(ErrCreateEnvironment, err)
	}

	fmt.Printf(FmtEnvironmentSuccess, params.Name, params.Namespace)
	return nil
}

// GetEnvironmentsForNamespace returns environments filtered by namespace
func (e *EnvironmentResource) GetEnvironmentsForNamespace(namespaceName string) ([]resources.ResourceWrapper[*openchoreov1alpha1.Environment], error) {
	// List all environments in the namespace
	allEnvironments, err := e.List()
	if err != nil {
		return nil, err
	}

	// Filter by namespace
	var environments []resources.ResourceWrapper[*openchoreov1alpha1.Environment]
	for _, wrapper := range allEnvironments {
		if wrapper.Resource.GetLabels()[constants.LabelNamespace] == namespaceName {
			environments = append(environments, wrapper)
		}
	}

	return environments, nil
}
