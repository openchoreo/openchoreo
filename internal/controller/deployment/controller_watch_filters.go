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

package deployment

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

type NonOwnedEndpointWatchFilter struct {
	labelKey string
}

func (r *NonOwnedEndpointWatchFilter) IsRelevant(obj client.Object) bool {
	endpoint, ok := obj.(*choreov1.Endpoint)
	if !ok {
		return false
	}

	// Check if it has owner references
	// If it has owner references, returning as it is not relevant
	if len(endpoint.GetOwnerReferences()) > 0 {
		return false
	}

	// Check if the label exists
	_, exists := endpoint.Labels[r.labelKey]
	return exists
}

func (r *NonOwnedEndpointWatchFilter) GetReconcileRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	endpoint, ok := obj.(*choreov1.Endpoint)
	if !ok {
		return nil
	}

	deploymentName, exists := endpoint.Labels[r.labelKey]
	if !exists {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name:      deploymentName,
				Namespace: endpoint.Namespace,
			},
		},
	}
}
