// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"fmt"
	"sync"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

// RemoveClient removes a client from the cache by key.
// This is useful when a DataPlane/WorkflowPlane CR is updated and the cached client needs to be invalidated.
func (m *KubeMultiClientManager) RemoveClient(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, key)
}

// GetK8sClientFromDataPlane retrieves a Kubernetes client from DataPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
// Note: Cache key includes CR for isolation, but planeIdentifier for proxy uses only planeID
func GetK8sClientFromDataPlane(
	clientMgr *KubeMultiClientManager,
	dataplane *openchoreov1alpha1.DataPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Determine effective planeID (defaults to CR name if not specified)
	planeID := dataplane.Spec.PlaneID
	if planeID == "" {
		planeID = dataplane.Name
	}

	// Cache key: CR-specific for client isolation (each CR gets its own client instance)
	// Include "v2" to force cache invalidation after proxy client signature change
	key := fmt.Sprintf("v2/dataplane/%s/%s/%s", planeID, dataplane.Namespace, dataplane.Name)

	// Plane identifier for proxy routing: simplified 2-part format
	// Gateway routes to agent using only planeType and planeID
	// CR info is sent in URL for metadata (logging, future authorization)
	planeIdentifier := fmt.Sprintf("dataplane/%s", planeID)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		// Proxy client needs CR namespace/name to construct full 6-part URL
		return NewProxyClient(gatewayURL, planeIdentifier, dataplane.Namespace, dataplane.Name, clientMgr.ProxyTLSConfig)
	})
}

// GetK8sClientFromWorkflowPlane retrieves a Kubernetes client from WorkflowPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
// Note: Cache key includes CR for isolation, but planeIdentifier for proxy uses only planeID
func GetK8sClientFromWorkflowPlane(
	clientMgr *KubeMultiClientManager,
	workflowPlane *openchoreov1alpha1.WorkflowPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Determine effective planeID (defaults to CR name if not specified)
	planeID := workflowPlane.Spec.PlaneID
	if planeID == "" {
		planeID = workflowPlane.Name
	}

	// Cache key: CR-specific for client isolation (each CR gets its own client instance)
	// Include "v2" to force cache invalidation after proxy client signature change
	key := fmt.Sprintf("v2/workflowplane/%s/%s/%s", planeID, workflowPlane.Namespace, workflowPlane.Name)

	// Plane identifier for proxy routing: simplified 2-part format
	// Gateway routes to agent using only planeType and planeID
	// CR info is sent in URL for metadata (logging, future authorization)
	planeIdentifier := fmt.Sprintf("workflowplane/%s", planeID)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		// Proxy client needs CR namespace/name to construct full 6-part URL
		return NewProxyClient(gatewayURL, planeIdentifier, workflowPlane.Namespace, workflowPlane.Name, clientMgr.ProxyTLSConfig)
	})
}

// GetK8sClientFromClusterWorkflowPlane retrieves a Kubernetes client from ClusterWorkflowPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
// Note: Cache key includes CR for isolation, but planeIdentifier for proxy uses only planeID
func GetK8sClientFromClusterWorkflowPlane(
	clientMgr *KubeMultiClientManager,
	clusterWorkflowPlane *openchoreov1alpha1.ClusterWorkflowPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Determine effective planeID (defaults to CR name if not specified)
	planeID := clusterWorkflowPlane.Spec.PlaneID
	if planeID == "" {
		planeID = clusterWorkflowPlane.Name
	}

	// Cache key: CR-specific for client isolation (cluster-scoped, no namespace)
	// Include "v2" to force cache invalidation after proxy client signature change
	key := fmt.Sprintf("v2/clusterworkflowplane/%s/%s", planeID, clusterWorkflowPlane.Name)

	// Plane identifier for proxy routing: same format as namespace-scoped WorkflowPlane
	// Agents register by planeType/planeID regardless of CR scope
	planeIdentifier := fmt.Sprintf("workflowplane/%s", planeID)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		// Cluster-scoped: use placeholder namespace to maintain 6-part URL format
		return NewProxyClient(gatewayURL, planeIdentifier, "_cluster", clusterWorkflowPlane.Name, clientMgr.ProxyTLSConfig)
	})
}

// GetK8sClientFromClusterDataPlane retrieves a Kubernetes client from ClusterDataPlane specification.
// Only supports cluster agent mode via HTTP proxy through cluster gateway.
// Note: Cache key includes CR for isolation, but planeIdentifier for proxy uses only planeID
func GetK8sClientFromClusterDataPlane(
	clientMgr *KubeMultiClientManager,
	clusterDataplane *openchoreov1alpha1.ClusterDataPlane,
	gatewayURL string,
) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required for cluster agent mode")
	}

	// Determine effective planeID (defaults to CR name if not specified)
	planeID := clusterDataplane.Spec.PlaneID
	if planeID == "" {
		planeID = clusterDataplane.Name
	}

	// Cache key: CR-specific for client isolation (cluster-scoped, no namespace)
	// Include "v2" to force cache invalidation after proxy client signature change
	key := fmt.Sprintf("v2/clusterdataplane/%s/%s", planeID, clusterDataplane.Name)

	// Plane identifier for proxy routing: same format as namespace-scoped DataPlane
	// Agents register by planeType/planeID regardless of CR scope
	planeIdentifier := fmt.Sprintf("dataplane/%s", planeID)

	// Use GetOrAddClient to cache the proxy client
	return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
		// Cluster-scoped: use placeholder namespace to maintain 6-part URL format
		return NewProxyClient(gatewayURL, planeIdentifier, "_cluster", clusterDataplane.Name, clientMgr.ProxyTLSConfig)
	})
}

// GetK8sClientFromObservabilityPlane retrieves a Kubernetes client from ObservabilityPlane specification.
// Currently only supports agent mode (via HTTP proxy through cluster gateway).
func GetK8sClientFromObservabilityPlane(
	clientMgr *KubeMultiClientManager,
	observabilityPlane *openchoreov1alpha1.ObservabilityPlane,
	gatewayURL string,
) (client.Client, error) {
	// Include plane type in cache key to avoid collision with DataPlane and WorkflowPlane
	// Include "v2" to force cache invalidation after proxy client signature change
	key := fmt.Sprintf("v2/observabilityplane/%s/%s", observabilityPlane.Namespace, observabilityPlane.Name)

	// Agent mode - use HTTP proxy through cluster gateway
	if observabilityPlane.Spec.ClusterAgent.ClientCA.Value != "" {
		if gatewayURL == "" {
			return nil, fmt.Errorf("gatewayURL is required for agent mode")
		}

		planeID := observabilityPlane.Spec.PlaneID
		if planeID == "" {
			planeID = observabilityPlane.Name
		}

		// Use planeType/planeName format to match agent registration
		// Agent registers as "observabilityplane/<name>", so we use the same identifier
		planeIdentifier := fmt.Sprintf("observabilityplane/%s", planeID)

		// Use GetOrAddClient to cache the proxy client
		return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
			return NewProxyClient(gatewayURL, planeIdentifier, observabilityPlane.Namespace, observabilityPlane.Name, clientMgr.ProxyTLSConfig)
		})
	}

	// ObservabilityPlane currently only supports agent mode
	return nil, fmt.Errorf("agent mode must be enabled for ObservabilityPlane")
}

// GetK8sClientFromClusterObservabilityPlane retrieves a Kubernetes client from ClusterObservabilityPlane specification.
// Only supports agent mode (via HTTP proxy through cluster gateway).
func GetK8sClientFromClusterObservabilityPlane(
	clientMgr *KubeMultiClientManager,
	clusterObsPlane *openchoreov1alpha1.ClusterObservabilityPlane,
	gatewayURL string,
) (client.Client, error) {
	// Include plane type in cache key to avoid collision with other plane types
	// Cluster-scoped: no namespace in key
	key := fmt.Sprintf("v2/clusterobservabilityplane/%s", clusterObsPlane.Name)

	// Agent mode - use HTTP proxy through cluster gateway
	if clusterObsPlane.Spec.ClusterAgent.ClientCA.Value != "" {
		if gatewayURL == "" {
			return nil, fmt.Errorf("gatewayURL is required for agent mode")
		}

		// Determine effective planeID (defaults to CR name if not specified)
		planeID := clusterObsPlane.Spec.PlaneID
		if planeID == "" {
			planeID = clusterObsPlane.Name
		}

		// Use planeType/planeName format to match agent registration
		planeIdentifier := fmt.Sprintf("observabilityplane/%s", planeID)

		// Use GetOrAddClient to cache the proxy client
		return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
			// Cluster-scoped: use placeholder namespace to maintain 6-part URL format
			return NewProxyClient(gatewayURL, planeIdentifier, "_cluster", clusterObsPlane.Name, clientMgr.ProxyTLSConfig)
		})
	}

	// ClusterObservabilityPlane currently only supports agent mode
	return nil, fmt.Errorf("agent mode must be enabled for ClusterObservabilityPlane")
}
