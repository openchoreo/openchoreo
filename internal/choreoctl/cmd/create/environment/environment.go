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

package environment

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/choreoctl/errors"
	"github.com/choreo-idp/choreo/internal/choreoctl/util"
	"github.com/choreo-idp/choreo/pkg/cli/common/constants"
	"github.com/choreo-idp/choreo/pkg/cli/types/api"
)

type CreateEnvironmentImpl struct {
	config constants.CRDConfig
}

func NewCreateEnvironmentImpl(config constants.CRDConfig) *CreateEnvironmentImpl {
	return &CreateEnvironmentImpl{
		config: config,
	}
}

func (i *CreateEnvironmentImpl) CreateEnvironment(params api.CreateEnvironmentParams) error {
	if params.Interactive {
		return createEnvironmentInteractive()
	}

	if err := util.ValidateParams(util.CmdCreate, util.ResourceEnvironment, params); err != nil {
		return err
	}

	return createEnvironment(params)
}

func createEnvironment(params api.CreateEnvironmentParams) error {
	k8sClient, err := util.GetKubernetesClient()
	if err != nil {
		return err
	}

	env := &corev1.Environment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.Name,
			Namespace: params.Organization,
			Annotations: map[string]string{
				"core.choreo.dev/display-name": params.DisplayName,
				"core.choreo.dev/description":  params.Description,
			},
			Labels: map[string]string{
				"core.choreo.dev/organization": params.Organization,
				"core.choreo.dev/name":         params.Name,
			},
		},
		Spec: corev1.EnvironmentSpec{
			DataPlaneRef: params.DataPlaneRef,
			IsProduction: params.IsProduction,
			Gateway: corev1.GatewayConfig{
				DNSPrefix: params.DNSPrefix,
			},
		},
	}

	if err := k8sClient.Create(context.Background(), env); err != nil {
		return errors.NewError("failed to create environment: %v", err)
	}

	fmt.Printf("Environment '%s' created successfully in organization '%s'\n", params.Name, params.Organization)
	return nil
}
