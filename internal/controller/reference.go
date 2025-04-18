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

package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/labels"
)

func GetDataplaneOfEnv(ctx context.Context, c client.Client, env *choreov1.Environment) (*choreov1.DataPlane, error) {
	dataplaneList := &choreov1.DataPlaneList{}
	listOpts := []client.ListOption{
		client.InNamespace(env.GetNamespace()),
		client.MatchingLabels{
			labels.LabelKeyOrganizationName: GetOrganizationName(env),
			labels.LabelKeyName:             env.Spec.DataPlaneRef,
		},
	}

	if err := c.List(ctx, dataplaneList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list dataplanes: %w", err)
	}

	if len(dataplaneList.Items) > 0 {
		return &dataplaneList.Items[0], nil
	}

	return nil, fmt.Errorf("failed to find dataplane for environment: %s", env.Spec.DataPlaneRef)
}
