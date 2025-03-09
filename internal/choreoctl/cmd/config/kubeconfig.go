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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configContext "github.com/choreo-idp/choreo/pkg/cli/cmd/config"
)

var (
	scheme = runtime.NewScheme()
)

// GetKubernetesClient returns a new kubernetes client based on the current context
func GetKubernetesClient() (client.Client, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, err
	}

	k8sClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return k8sClient, nil
}

// GetDefaultKubeconfigPath returns the default kubeconfig path
func GetDefaultKubeconfigPath() (string, error) {
	if kubeconfigPath := os.Getenv("KUBECONFIG"); kubeconfigPath != "" {
		return filepath.Abs(kubeconfigPath)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Abs(filepath.Join(homeDir, ".kube", "config"))
}

// GetDefaultKubeconfigWithContext returns the default kubeconfig path and its current context
func GetDefaultKubeconfigWithContext() (string, string, error) {
	kubeconfigPath, err := GetDefaultKubeconfigPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to get kubeconfig path: %w", err)
	}

	k8sConfig, err := clientcmd.LoadFromFile(kubeconfigPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	return kubeconfigPath, k8sConfig.CurrentContext, nil
}

// getKubernetesConfig returns the kubernetes rest config from the current context
func getKubernetesConfig() (*rest.Config, error) {
	kubeconfig, context, err := getStoredKubeConfigValues()
	if err != nil {
		return nil, err
	}

	return buildKubeConfig(kubeconfig, context)
}

// buildKubeConfig builds kubernetes rest config from kubeconfig path and context
func buildKubeConfig(kubeconfigPath, context string) (*rest.Config, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{
		ExplicitPath: kubeconfigPath,
	}

	configOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: context,
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		configOverrides,
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	return config, nil
}

// getStoredKubeConfigValues returns the kubeconfig path and context from the current context
func getStoredKubeConfigValues() (string, string, error) {
	// Load stored choreoctl config
	cfg, err := LoadStoredConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to load config: %w", err)
	}

	// Check for current context
	if cfg.CurrentContext == "" {
		return "", "", fmt.Errorf("no current context set")
	}

	// Find current context
	var currentContext *configContext.Context
	for _, ctx := range cfg.Contexts {
		if ctx.Name == cfg.CurrentContext {
			currentContext = &ctx
			break
		}
	}
	if currentContext == nil {
		return "", "", fmt.Errorf("current context %q not found", cfg.CurrentContext)
	}

	// Find referenced cluster
	if currentContext.ClusterRef == "" {
		return "", "", fmt.Errorf("no cluster reference in context %q", cfg.CurrentContext)
	}

	// Get cluster config
	for _, cluster := range cfg.Clusters {
		if cluster.Name == currentContext.ClusterRef {
			return cluster.Kubeconfig, cluster.Context, nil
		}
	}

	return "", "", fmt.Errorf("referenced cluster %q not found", currentContext.ClusterRef)
}

// GetStoredKubeConfigValues returns the kubeconfig path and context from the current context
func GetStoredKubeConfigValues() (string, string, error) {
	return getStoredKubeConfigValues()
}

// GetKubeContextNames returns a sorted list of Kubernetes context names
func GetKubeContextNames(config *clientcmdapi.Config) []string {
	contexts := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		contexts = append(contexts, name)
	}
	sort.Strings(contexts)
	return contexts
}
