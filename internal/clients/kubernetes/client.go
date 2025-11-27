// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	egv1a1 "github.com/envoyproxy/gateway/api/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwapiv1 "sigs.k8s.io/gateway-api/apis/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	clustergateway "github.com/openchoreo/openchoreo/internal/cluster-gateway"
	argo "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/argoproj.io/workflow/v1alpha1"
	ciliumv2 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/cilium.io/v2"
	csisecretv1 "github.com/openchoreo/openchoreo/internal/dataplane/kubernetes/types/secretstorecsi/v1"
)

// KubeMultiClientManager maintains a cache of Kubernetes clients keyed by a unique identifier.
// Uses RWMutex to allow concurrent reads while still protecting writes.
type KubeMultiClientManager struct {
	mu      sync.RWMutex
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

// GetClient returns an existing Kubernetes client or creates one using the provided cluster configuration.
func (m *KubeMultiClientManager) GetClient(key string, kubernetesCluster *openchoreov1alpha1.KubernetesClusterSpec) (client.Client, error) {
	return m.GetOrAddClient(key, func() (client.Client, error) {
		// Validate that kubernetesCluster is not nil
		if kubernetesCluster == nil {
			return nil, fmt.Errorf("kubernetesCluster configuration is required for direct access mode")
		}

		// Create REST config from the new structure
		restCfg, err := buildRESTConfig(*kubernetesCluster)
		if err != nil {
			return nil, fmt.Errorf("failed to build REST config: %w", err)
		}

		// Create the new client
		cl, err := client.New(restCfg, client.Options{Scheme: scheme.Scheme})
		if err != nil {
			return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
		}

		return cl, nil
	})
}

// buildRESTConfig constructs a REST config from the KubernetesClusterSpec
func buildRESTConfig(kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec) (*rest.Config, error) {
	restCfg := &rest.Config{
		Host: kubernetesCluster.Server,
	}

	// Configure TLS
	if err := configureTLS(restCfg, &kubernetesCluster.TLS); err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	// Configure authentication with priority: mTLS > bearerToken > OIDC
	if kubernetesCluster.Auth.MTLS != nil {
		if err := configureMTLSAuth(restCfg, kubernetesCluster.Auth.MTLS); err != nil {
			return nil, fmt.Errorf("failed to configure mTLS authentication: %w", err)
		}
	} else if kubernetesCluster.Auth.BearerToken != nil {
		if err := configureBearerAuth(restCfg, kubernetesCluster.Auth.BearerToken); err != nil {
			return nil, fmt.Errorf("failed to configure bearer token authentication: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no supported authentication method configured")
	}

	return restCfg, nil
}

// configureTLS sets up TLS configuration
func configureTLS(restCfg *rest.Config, tls *openchoreov1alpha1.KubernetesTLS) error {
	// Only handle inline CA data for now
	if tls != nil && tls.CA.Value != "" {
		// Decode base64 encoded CA certificate
		caCert, err := base64.StdEncoding.DecodeString(tls.CA.Value)
		if err != nil {
			return fmt.Errorf("failed to decode base64 CA certificate: %w", err)
		}
		restCfg.TLSClientConfig.CAData = caCert
	}

	return nil
}

// configureMTLSAuth sets up mutual TLS authentication
func configureMTLSAuth(restCfg *rest.Config, mtlsAuth *openchoreov1alpha1.MTLSAuth) error {
	if mtlsAuth == nil {
		return fmt.Errorf("mTLS authentication config is nil")
	}

	// Get client certificate (only inline value for now)
	if mtlsAuth.ClientCert.Value == "" {
		return fmt.Errorf("client certificate value is required for mTLS authentication")
	}
	clientCertData, err := base64.StdEncoding.DecodeString(mtlsAuth.ClientCert.Value)
	if err != nil {
		return fmt.Errorf("failed to decode base64 client certificate: %w", err)
	}

	// Get client key (only inline value for now)
	if mtlsAuth.ClientKey.Value == "" {
		return fmt.Errorf("client key value is required for mTLS authentication")
	}
	clientKeyData, err := base64.StdEncoding.DecodeString(mtlsAuth.ClientKey.Value)
	if err != nil {
		return fmt.Errorf("failed to decode base64 client key: %w", err)
	}

	restCfg.TLSClientConfig.CertData = clientCertData
	restCfg.TLSClientConfig.KeyData = clientKeyData

	return nil
}

// configureBearerAuth sets up bearer token authentication
func configureBearerAuth(restCfg *rest.Config, bearerAuth *openchoreov1alpha1.ValueFrom) error {
	if bearerAuth == nil {
		return fmt.Errorf("bearer token authentication config is nil")
	}

	// Only handle inline token value for now
	if bearerAuth.Value == "" {
		return fmt.Errorf("bearer token value is required for bearer token authentication")
	}

	restCfg.BearerToken = bearerAuth.Value
	return nil
}

// makeClientKey generates a unique key for the client cache.
func makeClientKey(orgName, name string) string {
	return fmt.Sprintf("%s/%s", orgName, name)
}

// GetK8sClient retrieves a Kubernetes client for the specified org and cluster.
// Deprecated: Use GetK8sClientFromDataPlane instead
func GetK8sClient(
	clientMgr *KubeMultiClientManager,
	orgName, name string,
	kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec,
) (client.Client, error) {
	key := makeClientKey(orgName, name)
	cl, err := clientMgr.GetClient(key, &kubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	return cl, nil
}

// GetK8sClientFromDataPlane retrieves a Kubernetes client from DataPlane specification.
// It automatically handles agent mode vs direct access mode.
func GetK8sClientFromDataPlane(
	clientMgr *KubeMultiClientManager,
	dataplane *openchoreov1alpha1.DataPlane,
	agentServer interface{}, // *server.Server, passed as interface to avoid circular dependency
) (client.Client, error) {
	key := makeClientKey(dataplane.Namespace, dataplane.Name)

	if dataplane.Spec.Agent != nil && dataplane.Spec.Agent.Enabled {
		// Agent mode - use GetOrAddClient to handle caching and locking
		return clientMgr.GetOrAddClient(key, func() (client.Client, error) {
			if agentServer == nil {
				return nil, fmt.Errorf("agent server is required for agent mode but not provided")
			}

			srv, ok := agentServer.(clustergateway.Dispatcher)
			if !ok {
				return nil, fmt.Errorf("invalid agent server type: expected clustergateway.Dispatcher")
			}

			// Construct composite plane identifier: {planeType}/{planeName}
			// PlaneType is always "dataplane" for DataPlane CRs
			planeIdentifier := fmt.Sprintf("dataplane/%s", dataplane.Name)

			// Create agent client with 30 second timeout for requests
			requestTimeout := 30 * time.Second
			cl, err := NewAgentClient(planeIdentifier, srv, requestTimeout)
			if err != nil {
				return nil, fmt.Errorf("failed to create agent client: %w", err)
			}

			return cl, nil
		})
	}

	// Direct access mode
	if dataplane.Spec.KubernetesCluster == nil {
		return nil, fmt.Errorf("kubernetesCluster configuration is required when agent mode is disabled")
	}

	cl, err := clientMgr.GetClient(key, dataplane.Spec.KubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	return cl, nil
}
