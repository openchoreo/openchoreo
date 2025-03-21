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

package component

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/choreo-idp/choreo/internal/controller"
)

// ReasonComponentCreated is the reason used when a component is created/ready
const ReasonComponentCreated controller.ConditionReason = "ComponentCreated"

// NewComponentCreatedCondition creates a condition to indicate the component is created/ready
func NewComponentCreatedCondition(generation int64) metav1.Condition {
	return controller.NewCondition(
		controller.TypeCreated,
		metav1.ConditionTrue,
		ReasonComponentCreated,
		"Component is created",
		generation,
	)
}
