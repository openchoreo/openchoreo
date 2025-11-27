// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustergateway

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/openchoreo/openchoreo/internal/cluster-agent/messaging"
)

// RemoteServerClient is an HTTP client that connects to a remote agent server
// This allows controllers to communicate with an agent server running in-cluster
// while the controller itself runs locally (useful for development)
type RemoteServerClient struct {
	serverURL  string
	httpClient *http.Client
}

// NewRemoteServerClient creates a new client that connects to a remote agent server
// For development use only - uses insecureSkipVerify
func NewRemoteServerClient(serverURL string, insecureSkipVerify bool) *RemoteServerClient {
	return &RemoteServerClient{
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // Intentional: for dev mode only
				},
			},
		},
	}
}

// NewRemoteServerClientWithConfig creates a new client with full TLS configuration
// This is the recommended method for production deployments
func NewRemoteServerClientWithConfig(config *RemoteServerClientConfig) (*RemoteServerClient, error) {
	// Default timeout
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.InsecureSkipVerify, //nolint:gosec // Configurable: user controls via config
		MinVersion:         tls.VersionTLS12,
	}

	if config.ServerCAPath != "" && !config.InsecureSkipVerify {
		caCert, err := os.ReadFile(config.ServerCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read server CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse server CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate and key for mTLS if provided
	if config.ClientCertPath != "" && config.ClientKeyPath != "" {
		clientCert, err := tls.LoadX509KeyPair(config.ClientCertPath, config.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}

		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return &RemoteServerClient{
		serverURL: config.ServerURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		},
	}, nil
}

// SendClusterAgentRequest sends a request to the remote agent server and waits for response
// This method signature matches Server.SendClusterAgentRequest so it can be used interchangeably
func (c *RemoteServerClient) SendClusterAgentRequest(
	planeName string,
	requestType messaging.RequestType,
	identifier string,
	payload map[string]interface{},
	timeout time.Duration,
) (*messaging.ClusterAgentResponse, error) {
	requestBody := map[string]interface{}{
		"action": identifier,
	}

	for k, v := range payload {
		requestBody[k] = v
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/k8s-resources/%s", c.serverURL, planeName)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if timeout > 0 {
		c.httpClient.Timeout = timeout
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	var apiResponse struct {
		Success  bool                            `json:"success"`
		Plane    string                          `json:"plane"`
		Action   string                          `json:"action"`
		Request  map[string]interface{}          `json:"request"`
		Response *messaging.ClusterAgentResponse `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Return the dispatched response
	// NOTE: We return the response even if HTTP status is not 200
	// The caller will check response.Status and response.Error to handle errors appropriately
	if apiResponse.Response == nil {
		return nil, fmt.Errorf("missing response in API response")
	}

	return apiResponse.Response, nil
}
