// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"fmt"
	"sync"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
)

// ProxyTLSConfig holds TLS configuration for HTTP proxy connections to the cluster gateway
type ProxyTLSConfig struct {
	CACertPath     string
	ClientCertPath string
	ClientKeyPath  string
}

// KubeMultiClientManager maintains a cache of Kubernetes clients keyed by a unique identifier.
// Uses RWMutex to allow concurrent reads while still protecting writes.
type KubeMultiClientManager struct {
	mu             sync.RWMutex
	clients        map[string]client.Client
	ProxyTLSConfig *ProxyTLSConfig // TLS configuration for HTTP proxy connections
}

// NewManager initializes a new KubeMultiClientManager.
func NewManager() *KubeMultiClientManager {
	return &KubeMultiClientManager{
		clients: make(map[string]client.Client),
	}
}

// NewManagerWithProxyTLS initializes a new KubeMultiClientManager with proxy TLS configuration.
func NewManagerWithProxyTLS(tlsConfig *ProxyTLSConfig) *KubeMultiClientManager {
	return &KubeMultiClientManager{
		clients:        make(map[string]client.Client),
		ProxyTLSConfig: tlsConfig,
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

// GetOrAddClient returns a cached client or creates one using the provided create function.
// This method encapsulates all locking logic, ensuring thread-safe access to the client cache.
// If a client exists for the given key, it returns immediately. Otherwise, it calls createFunc
// to create a new client, caches it, and returns it.
func (m *KubeMultiClientManager) GetOrAddClient(key string, createFunc func() (client.Client, error)) (client.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return cached client if it exists
	if cl, exists := m.clients[key]; exists {
		return cl, nil
	}

	// Create new client using the provided function
	cl, err := createFunc()
	if err != nil {
		return nil, err
	}

	// Cache and return the client
	m.clients[key] = cl
	return cl, nil
}

// GetK8sClientFromDataPlane retrieves a Kubernetes client from DataPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
func GetK8sClientFromDataPlane(
	clientMgr *KubeMultiClientManager,
	dataplane *openchoreov1alpha1.DataPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Include plane type in cache key to avoid collision with BuildPlane
	key := fmt.Sprintf("dataplane/%s/%s", dataplane.Namespace, dataplane.Name)

	// Use planeType/planeName format to match agent registration
	// Agent registers as "dataplane/<name>", so we use the same identifier
	planeIdentifier := fmt.Sprintf("dataplane/%s", dataplane.Name)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		return NewProxyClient(gatewayURL, planeIdentifier, clientMgr.ProxyTLSConfig)
	})
}

// GetK8sClientFromBuildPlane retrieves a Kubernetes client from BuildPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
func GetK8sClientFromBuildPlane(
	clientMgr *KubeMultiClientManager,
	buildplane *openchoreov1alpha1.BuildPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Include plane type in cache key to avoid collision with DataPlane
	key := fmt.Sprintf("buildplane/%s/%s", buildplane.Namespace, buildplane.Name)

	// Use planeType/planeName format to match agent registration
	// Agent registers as "buildplane/<name>", so we use the same identifier
	planeIdentifier := fmt.Sprintf("buildplane/%s", buildplane.Name)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		return NewProxyClient(gatewayURL, planeIdentifier, clientMgr.ProxyTLSConfig)
	})
}
