/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package kinds

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/choreoctl/resources"
	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/types/api"
)

// OrganizationResource provides operations for Organization CRs.
type OrganizationResource struct {
	*resources.BaseResource[*corev1.Organization, *corev1.OrganizationList]
}

// NewOrganizationResource constructs an OrganizationResource with only the CRDConfig.
func NewOrganizationResource(cfg constants.CRDConfig) (*OrganizationResource, error) {
	cli, err := resources.GetClient()
	if err != nil {
		return nil, fmt.Errorf(ErrCreateKubeClient, err)
	}

	return &OrganizationResource{
		BaseResource: resources.NewBaseResource[*corev1.Organization, *corev1.OrganizationList](
			resources.WithClient[*corev1.Organization, *corev1.OrganizationList](cli),
			resources.WithConfig[*corev1.Organization, *corev1.OrganizationList](cfg),
		),
	}, nil
}

// GetStatus returns the status of an Organization with detailed information.
func (o *OrganizationResource) GetStatus(org *corev1.Organization) string {
	return resources.GetReadyStatus(
		org.Status.Conditions,
		StatusPending,
		StatusReady,
		StatusNotReady,
	)
}

// GetAge returns the age of an Organization.
func (o *OrganizationResource) GetAge(org *corev1.Organization) string {
	return resources.FormatAge(org.GetCreationTimestamp().Time)
}

// PrintTableItems formats organizations into a table
func (o *OrganizationResource) PrintTableItems(orgs []resources.ResourceWrapper[*corev1.Organization]) error {
	if len(orgs) == 0 {
		fmt.Println("No organizations found")
		return nil
	}

	rows := make([][]string, 0, len(orgs))

	for _, wrapper := range orgs {
		org := wrapper.Resource
		displayName := org.GetAnnotations()[constants.AnnotationDisplayName]

		rows = append(rows, []string{
			resources.FormatNameWithDisplayName(wrapper.LogicalName, displayName),
			o.GetStatus(org),
			resources.FormatAge(org.GetCreationTimestamp().Time),
		})
	}
	return resources.PrintTable(HeadersOrganization, rows)
}

// Print overrides the base Print method to ensure our custom PrintTableItems is called
func (o *OrganizationResource) Print(format resources.OutputFormat, filter *resources.ResourceFilter) error {
	// List resources
	orgs, err := o.List()
	if err != nil {
		return err
	}

	// Apply name filter if specified
	if filter != nil && filter.Name != "" {
		filtered, err := resources.FilterByName(orgs, filter.Name)
		if err != nil {
			return err
		}
		orgs = filtered
	}

	// Call the appropriate print method based on format
	switch format {
	case resources.OutputFormatTable:
		return o.PrintTableItems(orgs)
	case resources.OutputFormatYAML:
		return o.BaseResource.PrintItems(orgs, format)
	default:
		return fmt.Errorf(ErrFormatUnsupported, format)
	}
}

// CreateOrganization creates a new Organization CR.
func (o *OrganizationResource) CreateOrganization(params api.CreateOrganizationParams) error {
	org := &corev1.Organization{
		ObjectMeta: metav1.ObjectMeta{
			Name: params.Name,
			Annotations: map[string]string{
				constants.AnnotationDisplayName: params.DisplayName,
				constants.AnnotationDescription: params.Description,
			},
			Labels: map[string]string{
				constants.LabelName:         params.Name,
				constants.LabelOrganization: params.Name,
			},
		},
	}
	if err := o.Create(org); err != nil {
		return fmt.Errorf(ErrCreateOrganization, err)
	}
	fmt.Printf(FmtOrganizationSuccess, params.Name)
	return nil
}
