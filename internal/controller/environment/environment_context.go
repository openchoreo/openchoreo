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

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
	"github.com/openchoreo/openchoreo/internal/controller"
	"github.com/openchoreo/openchoreo/internal/dataplane"
)

func (r *Reconciler) makeEnvironmentContext(ctx context.Context, environment *choreov1.Environment) (*dataplane.EnvironmentContext, error) {
	dataPlane, err := controller.GetDataPlane(ctx, r.Client, environment)
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve the dataplane: %w", err)
	}

	return &dataplane.EnvironmentContext{
		DataPlane:   dataPlane,
		Environment: environment,
	}, nil
}
