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
	restCfg := &rest.Config{
		Host: kubernetesCluster.Connection.Server,
	}

	// Configure TLS
	if err := configureTLS(restCfg, kubernetesCluster.Connection.TLS); err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	// Configure authentication based on type
	switch kubernetesCluster.Auth.Type {
	case openchoreov1alpha1.AuthTypeCert:
		if err := configureCertAuth(restCfg, kubernetesCluster.Auth.Cert); err != nil {
			return nil, fmt.Errorf("failed to configure certificate authentication: %w", err)
		}
	case openchoreov1alpha1.AuthTypeBearer:
		if err := configureBearerAuth(restCfg, kubernetesCluster.Auth.Bearer); err != nil {
			return nil, fmt.Errorf("failed to configure bearer authentication: %w", err)
		}
	case openchoreov1alpha1.AuthTypeOIDC:
		if err := configureOIDCAuth(restCfg, kubernetesCluster.Auth.OIDC); err != nil {
			return nil, fmt.Errorf("failed to configure OIDC authentication: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported authentication type: %s", kubernetesCluster.Auth.Type)
	}

	return restCfg, nil
}

// configureTLS sets up TLS configuration
func configureTLS(restCfg *rest.Config, tls openchoreov1alpha1.KubernetesTLS) error {
	// TODO: Add support for CASecretRef - fetch CA certificate from Kubernetes secret
	// when tls.CASecretRef is provided, retrieve the secret and extract the CA certificate

	// For now, only handle inline CA data (not secret refs)
	if tls.CAData != "" {
		caCert, err := base64.StdEncoding.DecodeString(tls.CAData)
		if err != nil {
			return fmt.Errorf("failed to decode CA cert: %w", err)
		}
		restCfg.TLSClientConfig.CAData = caCert
	}
	return nil
}

// configureCertAuth sets up certificate-based authentication
func configureCertAuth(restCfg *rest.Config, certAuth *openchoreov1alpha1.KubernetesCertAuth) error {
	if certAuth == nil {
		return fmt.Errorf("certificate authentication config is nil")
	}

	// TODO: Add support for ClientCertSecretRef and ClientKeySecretRef
	// Priority: 1. Secret references (ClientCertSecretRef, ClientKeySecretRef)
	//          2. Fallback to inline data (ClientCertData, ClientKeyData)

	// For now, only handle inline cert/key data (not secret refs)
	if certAuth.ClientCertData != "" && certAuth.ClientKeyData != "" {
		clientCert, err := base64.StdEncoding.DecodeString(certAuth.ClientCertData)
		if err != nil {
			return fmt.Errorf("failed to decode client cert: %w", err)
		}
		clientKey, err := base64.StdEncoding.DecodeString(certAuth.ClientKeyData)
		if err != nil {
			return fmt.Errorf("failed to decode client key: %w", err)
		}
		restCfg.TLSClientConfig.CertData = clientCert
		restCfg.TLSClientConfig.KeyData = clientKey
	} else {
		return fmt.Errorf("client certificate data and key data are required for certificate authentication")
	}

	return nil
}

// configureBearerAuth sets up bearer token authentication
func configureBearerAuth(restCfg *rest.Config, bearerAuth *openchoreov1alpha1.KubernetesBearerAuth) error {
	if bearerAuth == nil {
		return fmt.Errorf("bearer authentication config is nil")
	}

	// TODO: Add support for TokenSecretRef
	// Priority: 1. Secret reference (TokenSecretRef)
	//          2. Fallback to inline data (TokenData)

	// For now, only handle inline token data (not secret refs)
	if bearerAuth.TokenData != "" {
		restCfg.BearerToken = bearerAuth.TokenData
	} else {
		return fmt.Errorf("token data is required for bearer authentication")
	}

	return nil
}

// configureOIDCAuth sets up OIDC authentication
func configureOIDCAuth(restCfg *rest.Config, oidcAuth *openchoreov1alpha1.KubernetesOIDCAuth) error {
	if oidcAuth == nil {
		return fmt.Errorf("OIDC authentication config is nil")
	}

	// TODO: Implement OIDC authentication configuration
	// For now, this is a placeholder implementation
	// OIDC configuration would typically require more complex setup
	return fmt.Errorf("OIDC authentication is not yet implemented")
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
