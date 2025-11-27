// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// createKubernetesClient creates a Kubernetes client
// If kubeconfigPath is empty, it uses in-cluster config
// Otherwise, it loads the kubeconfig from the specified path
func createKubernetesClient(kubeconfigPath string) (k8sclient.Client, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w (use --kubeconfig for local development)", err)
		}
	} else {
		config, err = loadKubeconfig(kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}

	k8sClient, err := k8sclient.New(config, k8sclient.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return k8sClient, nil
}

func loadKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	return config, nil
}
