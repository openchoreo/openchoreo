// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// createKubernetesClient creates a Kubernetes client and returns the typed client,
// a dynamic client for arbitrary resources, and the rest config.
// If kubeconfigPath is empty, it uses in-cluster config
// Otherwise, it loads the kubeconfig from the specified path
func createKubernetesClient(kubeconfigPath string) (k8sclient.Client, dynamic.Interface, *rest.Config, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath == "" {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to get in-cluster config: %w (use --kubeconfig for local development)", err)
		}
	} else {
		config, err = loadKubeconfig(kubeconfigPath)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}
	}

	k8sClient, err := k8sclient.New(config, k8sclient.Options{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// The dynamic client backs resource tree child discovery, which lists
	// arbitrary GVRs named by control plane rules and so cannot use a scheme.
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create dynamic Kubernetes client: %w", err)
	}

	return k8sClient, dynamicClient, config, nil
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
