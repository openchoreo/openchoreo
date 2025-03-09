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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	choreov1 "github.com/choreo-idp/choreo/api/v1"
	"github.com/choreo-idp/choreo/internal/choreoctl/cmd/config"
)

// CreateNewScheme creates a new runtime scheme with Choreo CRDs registered
func CreateNewScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add core scheme: %w", err)
	}
	if err := choreov1.AddToScheme(s); err != nil {
		return nil, fmt.Errorf("failed to add Choreo scheme: %w", err)
	}
	return s, nil
}

// GetClient returns the controller-runtime client for CR operations
func GetClient() (client.Client, error) {
	kubeconfigPath, kubeContext, err := config.GetStoredKubeConfigValues()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig values: %w", err)
	}

	restConfig, err := buildKubeConfig(kubeconfigPath, kubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	c, err := client.New(restConfig, client.Options{
		Scheme: GetScheme(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return c, nil
}

// GetRESTConfig returns the REST config based on current context
func GetRESTConfig() (*rest.Config, error) {
	kubeconfigPath, kubeContext, err := config.GetStoredKubeConfigValues()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig values: %w", err)
	}

	return buildKubeConfig(kubeconfigPath, kubeContext)
}

// Private helper to build kubeconfig from path and context
func buildKubeConfig(kubeconfigPath, kubeContext string) (*rest.Config, error) {
	configLoader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	)

	config, err := configLoader.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	return config, nil
}
