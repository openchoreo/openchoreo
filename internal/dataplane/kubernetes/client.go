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
	"github.com/openchoreo/openchoreo/internal/labels"
)

type KubeClientManager struct {
	mu      sync.Mutex
	clients map[string]client.Client
}

// NewManager creates a manager with initialized client map
func NewManager() *KubeClientManager {
	return &KubeClientManager{
		clients: make(map[string]client.Client),
	}
}

// GetClient returns a cached clientset or creates a new one if not found
func (m *KubeClientManager) GetClient(key string, creds openchoreov1alpha1.APIServerCredentials) (client.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if the client is already cached
	if cl, exists := m.clients[key]; exists {
		return cl, nil
	}

	// Decode credentials
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

	// Build REST config
	restCfg := &rest.Config{
		Host: creds.APIServerURL,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   caCert,
			CertData: clientCert,
			KeyData:  clientKey,
		},
	}

	// Register API schemes
	_ = scheme.AddToScheme(scheme.Scheme)
	_ = ciliumv2.AddToScheme(scheme.Scheme)
	_ = gwapiv1.Install(scheme.Scheme)
	_ = egv1a1.AddToScheme(scheme.Scheme)
	_ = csisecretv1.Install(scheme.Scheme)
	_ = argo.AddToScheme(scheme.Scheme)

	// Create the client with the scheme
	cl, err := client.New(restCfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create dataplane client: %w", err)
	}

	// Cache the client
	m.clients[key] = cl
	return cl, nil
}

func makeDataplaneKey(dataplane *openchoreov1alpha1.DataPlane) string {
	return fmt.Sprintf("%s/%s", dataplane.Labels[labels.LabelKeyOrganizationName], dataplane.Labels[labels.LabelKeyName])
}

func GetDPClient(dpClientMgr *KubeClientManager, dataplane *openchoreov1alpha1.DataPlane) (client.Client, error) {
	// Get the DP client using the credentials from the dataplane kind
	dpClient, err := dpClientMgr.GetClient(makeDataplaneKey(dataplane), dataplane.Spec.KubernetesCluster.Credentials)
	if err != nil {
		// Return an error if client creation fails
		return nil, fmt.Errorf("failed to get DP client: %w", err)
	}

	// Return the DP client
	return dpClient, nil
}
