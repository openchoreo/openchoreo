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
