// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"encoding/base64"
	"fmt"
	"os"
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

// GetClient returns an existing Kubernetes client or creates one using the provided cluster configuration.
func (m *KubeMultiClientManager) GetClient(key string, kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec) (client.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return cached client if it exists
	if cl, exists := m.clients[key]; exists {
		return cl, nil
	}

	// Create REST config from the new structure
	restCfg, err := buildRESTConfig(kubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to build REST config: %w", err)
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

// buildRESTConfig constructs a REST config from the KubernetesClusterSpec
func buildRESTConfig(kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec) (*rest.Config, error) {
	// Determine the server URL
	var serverURL string
	if kubernetesCluster.Server != nil {
		serverURL = *kubernetesCluster.Server
	} else {
		// Use environment variables to dynamically configure the server URI
		kubeAPIIP := os.Getenv("KUBERNETES_SERVICE_HOST")
		kubeAPIPort := os.Getenv("KUBERNETES_SERVICE_PORT")
		if kubeAPIIP == "" || kubeAPIPort == "" {
			return nil, fmt.Errorf("KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT environment variables must be set when Server is not specified")
		}
		serverURL = fmt.Sprintf("https://%s:%s", kubeAPIIP, kubeAPIPort)
	}

	restCfg := &rest.Config{
		Host: serverURL,
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
func GetK8sClient(
	clientMgr *KubeMultiClientManager,
	orgName, name string,
	kubernetesCluster openchoreov1alpha1.KubernetesClusterSpec,
) (client.Client, error) {
	key := makeClientKey(orgName, name)
	cl, err := clientMgr.GetClient(key, kubernetesCluster)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes client: %w", err)
	}
	return cl, nil
}
