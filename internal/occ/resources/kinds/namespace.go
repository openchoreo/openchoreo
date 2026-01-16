// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openchoreo/openchoreo/internal/occ/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// NamespaceResource provides operations for Namespace resources.
type NamespaceResource struct {
	*resources.BaseResource[*corev1.Namespace, *corev1.NamespaceList]
}

// NewNamespaceResource constructs a NamespaceResource with only the CRDConfig.
func NewNamespaceResource(cfg constants.CRDConfig) (*NamespaceResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	return &NamespaceResource{
		BaseResource: resources.NewBaseResource[*corev1.Namespace, *corev1.NamespaceList](
			resources.WithClient[*corev1.Namespace, *corev1.NamespaceList](cli),
			resources.WithConfig[*corev1.Namespace, *corev1.NamespaceList](cfg),
		),
	}, nil
}

// GetStatus returns the status of a Namespace with detailed information.
func (o *NamespaceResource) GetStatus(namespace *corev1.Namespace) string {
	switch namespace.Status.Phase {
	case corev1.NamespaceActive:
		return StatusReady
	case corev1.NamespaceTerminating:
		return StatusNotReady
	default:
		return StatusPending
	}
}

// GetAge returns the age of a Namespace.
func (o *NamespaceResource) GetAge(namespace *corev1.Namespace) string {
	return resources.FormatAge(namespace.GetCreationTimestamp().Time)
}

// PrintTableItems formats namespaces into a table
func (o *NamespaceResource) PrintTableItems(namespaces []resources.ResourceWrapper[*corev1.Namespace]) error {
	if len(namespaces) == 0 {
		fmt.Println("No namespaces found")
		return nil
	}

	rows := make([][]string, 0, len(namespaces))

	for _, wrapper := range namespaces {
		namespace := wrapper.Resource
		displayName := namespace.GetAnnotations()[constants.AnnotationDisplayName]

		rows = append(rows, []string{
			resources.FormatNameWithDisplayName(wrapper.LogicalName, displayName),
			o.GetStatus(namespace),
			resources.FormatAge(namespace.GetCreationTimestamp().Time),
		})
	}
	return resources.PrintTable(HeadersNamespace, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (o *NamespaceResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	namespaces, err := o.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(namespaces, filter.Name)
		if err != nil {
			return err
		}
		namespaces = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return o.PrintTableItems(namespaces)
	case resources.OutputFormatYAML:
		return o.BaseResource.PrintItems(namespaces, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateNamespace creates a new Namespace.
func (o *NamespaceResource) CreateNamespace(params api.CreateNamespaceParams) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: params.Name,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: params.DisplayName,
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:      params.Name,
				constants.LabelNamespace: params.Name,
			},
		},
	}
	if err := o.Create(namespace); err != nil {
		return fmt.Errorf(ErrCreateNamespace, err)
	}
	fmt.Printf(FmtNamespaceSuccess, params.Name)
	return nil
}
