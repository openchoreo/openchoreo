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

package resources

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
)

var (
	// Rename from 'scheme' to 'schemeInstance' to avoid conflict
	schemeInstance = runtime.NewScheme()
)

func init() {
	// Register standard Kubernetes types
	utilruntime.Must(clientgoscheme.AddToScheme(schemeInstance))
	// Register Choreo CRDs
	utilruntime.Must(choreov1.AddToScheme(schemeInstance))
}

// GetScheme returns the runtime scheme with all required types registered
func GetScheme() *runtime.Scheme {
	return schemeInstance
}
