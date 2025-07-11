// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"encoding/base64"
	"fmt"
	"sync"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
)

// KubeMultiClientManager maintains a cache of Kubernetes clients keyed by a unique identifier.
type KubeMultiClientManager struct {
	mu      sync.Mutex
	clients map[string]client.Client
}

// NewManager initializes a new KubeMultiClientManager.
func NewManager() *KubeMultiClientManager {
	return &KubeMultiClientManager{
		clients: make(map[string]client.Client),
	}
}

func init() {
	_ = scheme.AddToScheme(scheme.Scheme)
	_ = openchoreov1alpha1.AddToScheme(scheme.Scheme)
	_ = ciliumv2.AddToScheme(scheme.Scheme)
	_ = gwapiv1.Install(scheme.Scheme)
	_ = egv1a1.AddToScheme(scheme.Scheme)
	_ = csisecretv1.Install(scheme.Scheme)
	_ = argo.AddToScheme(scheme.Scheme)
}

// GetClient returns an existing Kubernetes client or creates one using the provided credentials.
func (m *KubeMultiClientManager) GetClient(key string, creds openchoreov1alpha1.APIServerCredentials) (client.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return cached client if it exists
	if cl, exists := m.clients[key]; exists {
		return cl, nil
	}

	// Decode base64 credentials
	caCert, err := base64.StdEncoding.DecodeString(creds.CACert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode CA cert: %w", err)
	}
	clientCert, err := base64.StdEncoding.DecodeString(creds.ClientCert)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client cert: %w", err)
	}
	clientKey, err := base64.StdEncoding.DecodeString(creds.ClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode client key: %w", err)
	}

	// Construct REST config
	restCfg := &rest.Config{
		Host: creds.APIServerURL,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   caCert,
			CertData: clientCert,
			KeyData:  clientKey,
		},
	}

	// Create the new client
	cl, err := client.New(restCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Cache and return the client
	m.clients[key] = cl
	return cl, nil
}

// makeClientKey generates a unique key for the client cache.
func makeClientKey(orgName, name string) string {
	return fmt.Sprintf("%s/%s", orgName, name)
}

// GetK8sClient retrieves a Kubernetes client for the specified org and cluster.
func GetK8sClient(
	clientMgr *KubeMultiClientManager,
	orgName, name string,
	kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec,
) (client.Client, error) {
	key := makeClientKey(orgName, name)
	cl, err := clientMgr.GetClient(key, kubernetesCluster.Credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	return cl, nil
}
