// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// DataPlaneResource provides operations for DataPlane CRs.
type DataPlaneResource struct {
	*resources.BaseResource[*openchoreov1alpha1.DataPlane, *openchoreov1alpha1.DataPlaneList]
}

// NewDataPlaneResource constructs a DataPlaneResource with CRDConfig and optionally sets organization.
func NewDataPlaneResource(cfg constants.CRDConfig, org string) (*DataPlaneResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	options := []resources.ResourceOption[*openchoreov1alpha1.DataPlane, *openchoreov1alpha1.DataPlaneList]{
		resources.WithClient[*openchoreov1alpha1.DataPlane, *openchoreov1alpha1.DataPlaneList](cli),
		resources.WithConfig[*openchoreov1alpha1.DataPlane, *openchoreov1alpha1.DataPlaneList](cfg),
	}

	// Add organization namespace if provided
	if org != "" {
		options = append(options, resources.WithNamespace[*openchoreov1alpha1.DataPlane, *openchoreov1alpha1.DataPlaneList](org))
	}

	return &DataPlaneResource{
		BaseResource: resources.NewBaseResource(options...),
	}, nil
}

// WithNamespace sets the namespace for the dataplane resource (usually the organization name)
func (d *DataPlaneResource) WithNamespace(namespace string) {
	d.BaseResource.WithNamespace(namespace)
}

// GetStatus returns the status of a DataPlane with detailed information.
func (d *DataPlaneResource) GetStatus(dataPlane *openchoreov1alpha1.DataPlane) string {
	priorityConditions := []string{
		ConditionTypeReady,
		ConditionTypeAvailable,
		ConditionTypeConfigured,
	}

	return resources.GetResourceStatus(
		dataPlane.Status.Conditions,
		priorityConditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of a DataPlane.
func (d *DataPlaneResource) GetAge(dataPlane *openchoreov1alpha1.DataPlane) string {
	return resources.FormatAge(dataPlane.GetCreationTimestamp().Time)
}

// PrintTableItems formats dataplanes into a table
func (d *DataPlaneResource) PrintTableItems(dataPlanes []resources.ResourceWrapper[*openchoreov1alpha1.DataPlane]) error {
	if len(dataPlanes) == 0 {
		namespaceName := d.GetNamespace()

		message := "No data planes found"

		if namespaceName != "" {
			message += " in organization " + namespaceName
		}

		fmt.Println(message)
		return nil
	}

	rows := make([][]string, 0, len(dataPlanes))

	for _, wrapper := range dataPlanes {
		dataPlane := wrapper.Resource
		rows = append(rows, []string{
			wrapper.LogicalName,
			dataPlane.Spec.KubernetesCluster.Name,
			d.GetStatus(dataPlane),
			d.GetAge(dataPlane),
			dataPlane.GetLabels()[constants.LabelOrganization],
		})
	}
	return resources.PrintTable(HeadersDataPlane, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (d *DataPlaneResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	dataPlanes, err := d.List()
	if err != nil {
		return err
	}

	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(dataPlanes, filter.Name)
		if err != nil {
			return err
		}
		dataPlanes = filtered
	}

	switch format {
	case resources.OutputFormatTable:
		return d.PrintTableItems(dataPlanes)
	case resources.OutputFormatYAML:
		return d.BaseResource.PrintItems(dataPlanes, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateDataPlane creates a new DataPlane CR.
func (d *DataPlaneResource) CreateDataPlane(params api.CreateDataPlaneParams) error {
	k8sName := resources.GenerateResourceName(params.Organization, params.Name)

	// Create the DataPlane resource
	dataPlane := &openchoreov1alpha1.DataPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k8sName,
			Namespace: params.Organization,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: resources.DefaultIfEmpty(params.DisplayName, params.Name),
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:         params.Name,
				constants.LabelOrganization: params.Organization,
			},
		},
		Spec: openchoreov1alpha1.DataPlaneSpec{
			KubernetesCluster: openchoreov1alpha1.KubernetesClusterSpec{
				Name: params.KubernetesClusterName,
				Credentials: openchoreov1alpha1.APIServerCredentials{
					APIServerURL: params.APIServerURL,
					CACert:       params.CACert,
					ClientCert:   params.ClientCert,
					ClientKey:    params.ClientKey,
				},
			},
			Gateway: openchoreov1alpha1.GatewaySpec{
				PublicVirtualHost:       params.PublicVirtualHost,
				OrganizationVirtualHost: params.OrganizationVirtualHost,
			},
		},
	}

	// Create the dataplane using the base create method
	if err := d.Create(dataPlane); err != nil {
		return fmt.Errorf(ErrCreateDataPlane, err)
	}

	fmt.Printf(FmtDataPlaneCreateSuccess, params.Name, params.Organization)
	return nil
}

// GetDataPlanesForOrganization returns dataplanes filtered by organization
func (d *DataPlaneResource) GetDataPlanesForOrganization(orgName string) ([]resources.ResourceWrapper[*openchoreov1alpha1.DataPlane], error) {
	allDataPlanes, err := d.List()
	if err != nil {
		return nil, err
	}

	var dataPlanes []resources.ResourceWrapper[*openchoreov1alpha1.DataPlane]
	for _, wrapper := range allDataPlanes {
		if wrapper.Resource.GetLabels()[constants.LabelOrganization] == orgName {
			dataPlanes = append(dataPlanes, wrapper)
		}
	}

	return dataPlanes, nil
}
