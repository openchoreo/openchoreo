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

package deploymentpipeline

import (
	"fmt"

	"github.com/choreo-idp/choreo/internal/choreoctl/resources/kinds"
	"github.com/choreo-idp/choreo/internal/choreoctl/validation"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

// CreateDeploymentPipelineImpl implements the CreateDeploymentPipeline command
type CreateDeploymentPipelineImpl struct {
	config constants.CRDConfig
}

// NewCreateDeploymentPipelineImpl creates a new CreateDeploymentPipelineImpl instance
func NewCreateDeploymentPipelineImpl(config constants.CRDConfig) *CreateDeploymentPipelineImpl {
	return &CreateDeploymentPipelineImpl{
		config: config,
	}
}

// CreateDeploymentPipeline creates a new deployment pipeline based on provided parameters
func (i *CreateDeploymentPipelineImpl) CreateDeploymentPipeline(params api.CreateDeploymentPipelineParams) error {
	if err := validation.ValidateParams(validation.CmdCreate, validation.ResourceDeploymentPipeline, params); err != nil {
		return err
	}

	return createDeploymentPipeline(params, i.config)
}

// createDeploymentPipeline handles the creation of the deployment pipeline resource
func createDeploymentPipeline(params api.CreateDeploymentPipelineParams, config constants.CRDConfig) error {
	// Check for empty promotion paths and create a default one if needed
	if len(params.PromotionPaths) == 0 {
		// Try to get available environments to create a default promotion path
		envResource, err := kinds.NewEnvironmentResource(constants.EnvironmentV1Config, params.Organization)
		if err != nil {
			return fmt.Errorf("failed to create Environment resource: %w", err)
		}

		envs, err := envResource.List()
		if err != nil {
			return fmt.Errorf("failed to list environments: %w", err)
		}

		if len(envs) < 2 {
			return fmt.Errorf("at least two environments are required to create promotion paths")
		}

		// Create environment order either from provided order or from existing environments
		var envOrder []string

		if len(params.EnvironmentOrder) >= 2 {
			// Use user-provided environment order
			// Validate that the provided environment names actually exist
			envMap := make(map[string]bool)
			for _, env := range envs {
				envMap[env.LogicalName] = true
			}

			// Verify all environments in order exist
			for _, envName := range params.EnvironmentOrder {
				if !envMap[envName] {
					return fmt.Errorf("environment '%s' specified in environment-order does not exist", envName)
				}
			}

			envOrder = params.EnvironmentOrder
		} else {
			// Use the first two environments from the available ones
			// Order by creation timestamp as a sensible default
			envOrder = []string{
				envs[0].LogicalName,
				envs[1].LogicalName,
			}
			fmt.Println("No environment order specified. Using default order based on existing environments.")
		}

		// Create promotion paths based on the standard pattern found in the samples
		params.PromotionPaths = []api.PromotionPathParams{}

		// For environments in a sequence (like dev → staging → prod)
		// 1. First environment promotes to all others
		// 2. Each middle environment promotes to the ones after it
		for i := 0; i < len(envOrder)-1; i++ {
			sourceEnv := envOrder[i]
			targetEnvs := []api.TargetEnvironmentParams{}

			// Add each successive environment as a target
			for j := i + 1; j < len(envOrder); j++ {
				// For production targets from any source, require manual approval
				isProduction := (envOrder[j] == "production" || envOrder[j] == "prod")
				requiresManualApproval := isProduction && j > i+1 // Skip for direct next environment

				targetEnvs = append(targetEnvs, api.TargetEnvironmentParams{
					Name:                     envOrder[j],
					RequiresApproval:         true,
					IsManualApprovalRequired: requiresManualApproval,
				})
			}

			// Add this promotion path
			params.PromotionPaths = append(params.PromotionPaths, api.PromotionPathParams{
				SourceEnvironment:  sourceEnv,
				TargetEnvironments: targetEnvs,
			})
		}
	}

	pipelineRes, err := kinds.NewDeploymentPipelineResource(config, params.Organization)
	if err != nil {
		return fmt.Errorf("failed to create DeploymentPipeline resource: %w", err)
	}

	if err := pipelineRes.CreateDeploymentPipeline(params); err != nil {
		return fmt.Errorf("failed to create deployment pipeline '%s' in organization '%s': %w",
			params.Name, params.Organization, err)
	}

	return nil
}
