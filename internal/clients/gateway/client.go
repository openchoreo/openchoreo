// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

type Config struct {
	BaseURL string
	TLS     TLSConfig
	Timeout time.Duration
}

type TLSConfig struct {
	InsecureSkipVerify bool
	CAFile             string
	CAData             []byte
	ServerName         string
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type PlaneNotification struct {
	PlaneType string `json:"planeType"` // "dataplane", "buildplane", "observabilityplane"
	PlaneID   string `json:"planeID"`
	Event     string `json:"event"` // "created", "updated", "deleted"
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type NotificationResponse struct {
	DisconnectedAgents int  `json:"disconnectedAgents"`
	Success            bool `json:"success"`
}

type PlaneConnectionStatus struct {
	PlaneType       string    `json:"planeType"`
	PlaneID         string    `json:"planeID"`
	Connected       bool      `json:"connected"`
	ConnectedAgents int       `json:"connectedAgents"`
	LastSeen        time.Time `json:"lastSeen,omitempty"`
}

// TransientError represents a transient error that should be retried
// Examples: network errors, 5xx status codes, timeouts
type TransientError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *TransientError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("transient gateway error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("transient gateway error: %s", e.Message)
}

func (e *TransientError) Unwrap() error {
	return e.Err
}

// PermanentError represents a permanent error that should not be retried
// Examples: 4xx status codes (except 429), validation errors
type PermanentError struct {
	StatusCode int
	Message    string
}

func (e *PermanentError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("permanent gateway error (status %d): %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("permanent gateway error: %s", e.Message)
}

func IsTransientError(err error) bool {
	var transientErr *TransientError
	return err != nil && (errors.As(err, &transientErr) || isNetworkError(err))
}

func IsPermanentError(err error) bool {
	var permanentErr *PermanentError
	return err != nil && errors.As(err, &permanentErr)
}

func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "i/o timeout") ||
		errors.Is(err, context.DeadlineExceeded)
}

func classifyHTTPError(statusCode int) error {
	if statusCode >= http.StatusInternalServerError && statusCode < 600 {
		return &TransientError{
			StatusCode: statusCode,
			Message:    "gateway server error",
		}
	} else if statusCode == http.StatusTooManyRequests {
		return &TransientError{
			StatusCode: statusCode,
			Message:    "gateway rate limited",
		}
	} else if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
		return &PermanentError{
			StatusCode: statusCode,
			Message:    "gateway client error",
		}
	}
	return &TransientError{
		StatusCode: statusCode,
		Message:    "unexpected status code",
	}
}

func HandleGatewayError(logger interface{ Error(error, string, ...any) }, err error, operation string) (shouldRetry bool, result ctrl.Result, retryErr error) {
	if IsTransientError(err) {
		logger.Error(err, fmt.Sprintf("Transient error notifying gateway of %s, will retry", operation))
		return true, ctrl.Result{}, err
	}

	if IsPermanentError(err) {
		logger.Error(err, fmt.Sprintf("Permanent error notifying gateway of %s, skipping retry", operation))
		return false, ctrl.Result{}, nil
	}

	logger.Error(err, fmt.Sprintf("Failed to notify gateway of %s", operation))
	return false, ctrl.Result{}, nil
}

// NewClient creates a new gateway client with insecure TLS (for local development only)
// For production use, use NewClientWithConfig with proper TLS configuration
func NewClient(baseURL string) *Client {
	// Skip TLS verification for local development
	// In production, use NewClientWithConfig with proper CA certificates
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// #nosec G402 -- InsecureSkipVerify is intentional for local development
			// In production deployments, use NewClientWithConfig with proper CA certificates
			InsecureSkipVerify: true,
		},
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

// NewClientWithConfig creates a new gateway client with the provided configuration
// This should be used for production deployments with proper TLS verification
func NewClientWithConfig(config *Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}

	tlsConfig, err := buildTLSConfig(&config.TLS)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &Client{
		baseURL: config.BaseURL,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

func buildTLSConfig(config *TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12, // Enforce minimum TLS 1.2
	}

	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}

	if config.InsecureSkipVerify {
		// #nosec G402 -- InsecureSkipVerify is configurable and should only be used in development
		tlsConfig.InsecureSkipVerify = true
		return tlsConfig, nil
	}

	if config.CAFile != "" || len(config.CAData) > 0 {
		caCertPool := x509.NewCertPool()

		var caData []byte
		var err error

		if config.CAFile != "" {
			caData, err = os.ReadFile(config.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA file %s: %w", config.CAFile, err)
			}
		} else {
			caData = config.CAData
		}

		if !caCertPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

func (c *Client) NotifyPlaneLifecycle(ctx context.Context, notification *PlaneNotification) (*NotificationResponse, error) {
	body, err := json.Marshal(notification)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/planes/notify", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are transient and should be retried
		return nil, &TransientError{
			Message: "failed to send notification",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp.StatusCode)
	}

	var response NotificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

func (c *Client) ForceReconnect(ctx context.Context, planeType, planeID string) (*NotificationResponse, error) {
	url := fmt.Sprintf("%s/api/v1/planes/%s/%s/reconnect", c.baseURL, planeType, planeID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are transient and should be retried
		return nil, &TransientError{
			Message: "failed to send reconnect request",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp.StatusCode)
	}

	var response NotificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetPlaneStatus retrieves the connection status for a specific plane from the gateway
// This is used by controllers to query agent connection status and update CR status fields
// If namespace and name are provided, it returns CR-specific authorization status
// If they are empty, it returns plane-level connection status
func (c *Client) GetPlaneStatus(ctx context.Context, planeType, planeID, namespace, name string) (*PlaneConnectionStatus, error) {
	url := fmt.Sprintf("%s/api/v1/planes/%s/%s/status", c.baseURL, planeType, planeID)

	// Add query parameters for CR-specific status if provided
	if namespace != "" && name != "" {
		url = fmt.Sprintf("%s?namespace=%s&name=%s", url, namespace, name)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are transient and should be retried
		return nil, &TransientError{
			Message: "failed to get plane status",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyHTTPError(resp.StatusCode)
	}

	var status PlaneConnectionStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &status, nil
}

// PodReference represents a Kubernetes pod reference
type PodReference struct {
	Namespace string
	Name      string
}

// PodLogsOptions contains options for fetching pod logs through the gateway
type PodLogsOptions struct {
	ContainerName     string // Specific container name to get logs from
	IncludeTimestamps bool   // Include timestamps in log lines
	SinceSeconds      *int64 // Return logs newer than this many seconds
}

// GetPodLogsFromPlane retrieves pod logs through the gateway proxy with optional parameters
// This method makes direct Kubernetes API calls through the gateway proxy to support
// advanced log retrieval options like container selection, timestamps, and time filtering
func (c *Client) GetPodLogsFromPlane(ctx context.Context, planeType, planeID, planeNamespace, planeName string, podReference *PodReference, options *PodLogsOptions) (string, error) {
	const maxPodLogsBytes = 10 * 1024 * 1024 // 10MB. TODO: Make this configurable.

	if podReference == nil || podReference.Namespace == "" || podReference.Name == "" {
		return "", fmt.Errorf("pod reference is required and must have namespace and name")
	}

	// Build query parameters
	queryParams := ""
	if options != nil {
		params := []string{}
		if options.ContainerName != "" {
			params = append(params, fmt.Sprintf("container=%s", options.ContainerName))
		}
		if options.IncludeTimestamps {
			params = append(params, "timestamps=true")
		}
		if options.SinceSeconds != nil && *options.SinceSeconds > 0 {
			params = append(params, fmt.Sprintf("sinceSeconds=%d", *options.SinceSeconds))
		}
		if len(params) > 0 {
			queryParams = "?" + strings.Join(params, "&")
		}
	}

	// Build the URL for the Kubernetes API request
	k8sAPIPathForPodLogs := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/log%s", podReference.Namespace, podReference.Name, queryParams)
	k8sProxyURLForPodLogs := fmt.Sprintf("%s/api/proxy/%s/%s/%s/%s/k8s%s", c.baseURL, planeType, planeID, planeNamespace, planeName, k8sAPIPathForPodLogs)

	req, err := http.NewRequestWithContext(ctx, "GET", k8sProxyURLForPodLogs, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are transient and should be retried
		return "", &TransientError{
			Message: "failed to send request",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", classifyHTTPError(resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPodLogsBytes+1))
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if len(body) > maxPodLogsBytes {
		return "", fmt.Errorf("response body is too large, max is %d bytes", maxPodLogsBytes)
	}

	return string(body), nil
}
