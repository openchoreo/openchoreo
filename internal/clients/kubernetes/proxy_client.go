// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ProxyClient is a Kubernetes client that communicates through the cluster gateway HTTP proxy
// It implements the controller-runtime client.Client interface
type ProxyClient struct {
	gatewayURL  string
	planeType   string
	planeID     string
	crNamespace string
	crName      string
	httpClient  *http.Client
	scheme      *runtime.Scheme
}

// NewProxyClient creates a new proxy client for accessing a data plane or build plane through the cluster gateway
// planeIdentifier format: "planeType/planeID" (e.g., "dataplane/prod-cluster")
func NewProxyClient(gatewayURL, planeIdentifier string, crNamespace, crName string, tlsConfig *ProxyTLSConfig) (client.Client, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gatewayURL is required")
	}
	if planeIdentifier == "" {
		return nil, fmt.Errorf("planeIdentifier is required")
	}
	if crName == "" {
		return nil, fmt.Errorf("crName is required")
	}

	// Parse planeIdentifier: "planeType/planeID"
	parts := strings.Split(planeIdentifier, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid planeIdentifier format: expected 'planeType/planeID', got '%s'", planeIdentifier)
	}
	planeType := parts[0]
	planeID := parts[1]

	// Configure TLS for the HTTP client
	tlsCfg, err := buildProxyTLSConfig(tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS config: %w", err)
	}

	return &ProxyClient{
		gatewayURL:  strings.TrimSuffix(gatewayURL, "/"),
		planeType:   planeType,
		planeID:     planeID,
		crNamespace: crNamespace,
		crName:      crName,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsCfg,
			},
		},
		scheme: scheme.Scheme,
	}, nil
}

// buildProxyURL constructs the proxy URL in the new 6-part format:
// /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path}
func (pc *ProxyClient) buildProxyURL(apiPath string) string {
	// URL-encode path components to handle special characters
	encodedPlaneType := url.PathEscape(pc.planeType)
	encodedPlaneID := url.PathEscape(pc.planeID)
	encodedNamespace := url.PathEscape(pc.crNamespace)
	encodedCRName := url.PathEscape(pc.crName)

	// Target is always "k8s" for Kubernetes API requests
	// The full format is: /api/proxy/{planeType}/{planeID}/{namespace}/{crName}/{target}/{path}
	// Where {path} is the Kubernetes API path (e.g., /api/v1/namespaces/...)
	return fmt.Sprintf("%s/api/proxy/%s/%s/%s/%s/k8s%s",
		pc.gatewayURL, encodedPlaneType, encodedPlaneID, encodedNamespace, encodedCRName, apiPath)
}

// Get retrieves an object from the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	gvk, err := getGVK(obj, pc.scheme)
	if err != nil {
		return err
	}

	apiPath := pc.buildGetPath(gvk, key.Namespace, key.Name)

	proxyURL := pc.buildProxyURL(apiPath)

	req, err := http.NewRequestWithContext(ctx, "GET", proxyURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pc.handleErrorResponse(resp, gvk, key.Name)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// List retrieves a list of objects from the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOpts)
	}

	gvk, err := getGVKForList(list, pc.scheme)
	if err != nil {
		return err
	}

	apiPath := pc.buildListPath(gvk, listOpts.Namespace)

	queryParams := pc.buildListQueryParams(listOpts)

	proxyURL := pc.buildProxyURL(apiPath)
	if queryParams != "" {
		proxyURL += "?" + queryParams
	}

	req, err := http.NewRequestWithContext(ctx, "GET", proxyURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pc.handleErrorResponse(resp, gvk, "")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(body, list); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Create creates an object in the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	gvk, err := getGVK(obj, pc.scheme)
	if err != nil {
		return err
	}

	apiPath := pc.buildCreatePath(gvk, obj.GetNamespace())

	proxyURL := pc.buildProxyURL(apiPath)

	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", proxyURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pc.handleErrorResponse(resp, gvk, obj.GetName())
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(respBody, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Update updates an object in the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	gvk, err := getGVK(obj, pc.scheme)
	if err != nil {
		return err
	}

	apiPath := pc.buildUpdatePath(gvk, obj.GetNamespace(), obj.GetName())

	proxyURL := pc.buildProxyURL(apiPath)

	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", proxyURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pc.handleErrorResponse(resp, gvk, obj.GetName())
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(respBody, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Patch patches an object in the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	gvk, err := getGVK(obj, pc.scheme)
	if err != nil {
		return err
	}

	patchOpts := &client.PatchOptions{}
	for _, opt := range opts {
		opt.ApplyToPatch(patchOpts)
	}

	apiPath := pc.buildUpdatePath(gvk, obj.GetNamespace(), obj.GetName())

	queryParams := pc.buildPatchQueryParams(patchOpts)

	proxyURL := pc.buildProxyURL(apiPath)
	if queryParams != "" {
		proxyURL += "?" + queryParams
	}

	patchData, err := patch.Data(obj)
	if err != nil {
		return fmt.Errorf("failed to get patch data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", proxyURL, bytes.NewReader(patchData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", string(patch.Type()))
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return pc.handleErrorResponse(resp, gvk, obj.GetName())
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(respBody, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Delete deletes an object from the data plane Kubernetes API via the cluster gateway
func (pc *ProxyClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	gvk, err := getGVK(obj, pc.scheme)
	if err != nil {
		return err
	}

	apiPath := pc.buildDeletePath(gvk, obj.GetNamespace(), obj.GetName())

	proxyURL := pc.buildProxyURL(apiPath)

	req, err := http.NewRequestWithContext(ctx, "DELETE", proxyURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := pc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return pc.handleErrorResponse(resp, gvk, obj.GetName())
	}

	return nil
}

// DeleteAllOf deletes all objects matching the given list options
func (pc *ProxyClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return fmt.Errorf("DeleteAllOf not implemented for ProxyClient")
}

// Status returns a StatusWriter for updating object status
func (pc *ProxyClient) Status() client.StatusWriter {
	return &proxyStatusWriter{client: pc}
}

// SubResource returns a SubResourceClient for accessing subresources
func (pc *ProxyClient) SubResource(subResource string) client.SubResourceClient {
	return &proxySubResourceClient{
		client:      pc,
		subResource: subResource,
	}
}

// Scheme returns the scheme this client is using
func (pc *ProxyClient) Scheme() *runtime.Scheme {
	return pc.scheme
}

// RESTMapper returns the REST mapper
func (pc *ProxyClient) RESTMapper() meta.RESTMapper {
	// TODO: Implement proper REST mapper if needed
	return nil
}

// GroupVersionKindFor returns the GroupVersionKind for the given object
func (pc *ProxyClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return getGVK(obj, pc.scheme)
}

// IsObjectNamespaced returns true if the object is namespaced
func (pc *ProxyClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	gvk, err := pc.GroupVersionKindFor(obj)
	if err != nil {
		return false, err
	}
	return isNamespaced(gvk), nil
}

// proxyStatusWriter implements client.StatusWriter for the proxy client
type proxyStatusWriter struct {
	client *ProxyClient
}

// Create is not supported for status subresources
func (psw *proxyStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return fmt.Errorf("Create not supported for status subresource")
}

// Update updates the status subresource
func (psw *proxyStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	gvk, err := getGVK(obj, psw.client.scheme)
	if err != nil {
		return err
	}

	apiPath := psw.client.buildStatusPath(gvk, obj.GetNamespace(), obj.GetName())

	proxyURL := psw.client.buildProxyURL(apiPath)

	body, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", proxyURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := psw.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return psw.client.handleErrorResponse(resp, gvk, obj.GetName())
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(respBody, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// Patch patches the status subresource
func (psw *proxyStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	gvk, err := getGVK(obj, psw.client.scheme)
	if err != nil {
		return err
	}

	patchOpts := &client.SubResourcePatchOptions{}
	for _, opt := range opts {
		opt.ApplyToSubResourcePatch(patchOpts)
	}

	apiPath := psw.client.buildStatusPath(gvk, obj.GetNamespace(), obj.GetName())

	queryParams := psw.client.buildSubResourcePatchQueryParams(patchOpts)

	proxyURL := psw.client.buildProxyURL(apiPath)
	if queryParams != "" {
		proxyURL += "?" + queryParams
	}

	patchData, err := patch.Data(obj)
	if err != nil {
		return fmt.Errorf("failed to get patch data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PATCH", proxyURL, bytes.NewReader(patchData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", string(patch.Type()))
	req.Header.Set("Accept", "application/json")

	resp, err := psw.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return psw.client.handleErrorResponse(resp, gvk, obj.GetName())
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(respBody, obj); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// proxySubResourceClient implements client.SubResourceClient
type proxySubResourceClient struct {
	client      *ProxyClient
	subResource string
}

// Get retrieves a subresource
func (psr *proxySubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return fmt.Errorf("SubResource.Get not implemented for ProxyClient")
}

// Create creates a subresource
func (psr *proxySubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return fmt.Errorf("SubResource.Create not implemented for ProxyClient")
}

// Update updates a subresource
func (psr *proxySubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return fmt.Errorf("SubResource.Update not implemented for ProxyClient")
}

// Patch patches a subresource
func (psr *proxySubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return fmt.Errorf("SubResource.Patch not implemented for ProxyClient")
}

// buildGetPath builds the Kubernetes API path for a GET request
func (pc *ProxyClient) buildGetPath(gvk schema.GroupVersionKind, namespace, name string) string {
	resource := pluralizeKind(gvk.Kind)

	// Core API group (empty string)
	if gvk.Group == "" {
		if namespace != "" && isNamespaced(gvk) {
			return fmt.Sprintf("/api/%s/namespaces/%s/%s/%s", gvk.Version, namespace, resource, name)
		}
		return fmt.Sprintf("/api/%s/%s/%s", gvk.Version, resource, name)
	}

	// Non-core API groups (e.g., apps, batch, networking.k8s.io)
	if namespace != "" && isNamespaced(gvk) {
		return fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s/%s", gvk.Group, gvk.Version, namespace, resource, name)
	}
	return fmt.Sprintf("/apis/%s/%s/%s/%s", gvk.Group, gvk.Version, resource, name)
}

// buildListPath builds the Kubernetes API path for a LIST request
func (pc *ProxyClient) buildListPath(gvk schema.GroupVersionKind, namespace string) string {
	resource := pluralizeKind(gvk.Kind)

	// Core API group (empty string)
	if gvk.Group == "" {
		if namespace != "" && isNamespaced(gvk) {
			return fmt.Sprintf("/api/%s/namespaces/%s/%s", gvk.Version, namespace, resource)
		}
		return fmt.Sprintf("/api/%s/%s", gvk.Version, resource)
	}

	// Non-core API groups (e.g., apps, batch, networking.k8s.io)
	if namespace != "" && isNamespaced(gvk) {
		return fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s", gvk.Group, gvk.Version, namespace, resource)
	}
	return fmt.Sprintf("/apis/%s/%s/%s", gvk.Group, gvk.Version, resource)
}

// buildCreatePath builds the Kubernetes API path for a CREATE request
func (pc *ProxyClient) buildCreatePath(gvk schema.GroupVersionKind, namespace string) string {
	return pc.buildListPath(gvk, namespace)
}

// buildUpdatePath builds the Kubernetes API path for an UPDATE request
func (pc *ProxyClient) buildUpdatePath(gvk schema.GroupVersionKind, namespace, name string) string {
	return pc.buildGetPath(gvk, namespace, name)
}

// buildDeletePath builds the Kubernetes API path for a DELETE request
func (pc *ProxyClient) buildDeletePath(gvk schema.GroupVersionKind, namespace, name string) string {
	return pc.buildGetPath(gvk, namespace, name)
}

// buildStatusPath builds the Kubernetes API path for status subresource
func (pc *ProxyClient) buildStatusPath(gvk schema.GroupVersionKind, namespace, name string) string {
	return pc.buildGetPath(gvk, namespace, name) + "/status"
}

// buildListQueryParams builds query parameters for list requests
func (pc *ProxyClient) buildListQueryParams(opts *client.ListOptions) string {
	params := []string{}

	if opts.LabelSelector != nil {
		params = append(params, "labelSelector="+opts.LabelSelector.String())
	}

	if opts.FieldSelector != nil {
		params = append(params, "fieldSelector="+opts.FieldSelector.String())
	}

	if opts.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", opts.Limit))
	}

	if opts.Continue != "" {
		params = append(params, "continue="+opts.Continue)
	}

	return strings.Join(params, "&")
}

// buildPatchQueryParams builds query parameters for patch requests
func (pc *ProxyClient) buildPatchQueryParams(opts *client.PatchOptions) string {
	params := []string{}

	// fieldManager is required for Server-Side Apply
	if opts.FieldManager != "" {
		params = append(params, "fieldManager="+opts.FieldManager)
	}

	// force is used to force ownership transfer for Server-Side Apply
	if opts.Force != nil && *opts.Force {
		params = append(params, "force=true")
	}

	return strings.Join(params, "&")
}

// buildSubResourcePatchQueryParams builds query parameters for subresource patch requests
func (pc *ProxyClient) buildSubResourcePatchQueryParams(opts *client.SubResourcePatchOptions) string {
	params := []string{}

	// fieldManager is required for Server-Side Apply
	if opts.FieldManager != "" {
		params = append(params, "fieldManager="+opts.FieldManager)
	}

	// force is used to force ownership transfer for Server-Side Apply
	if opts.Force != nil && *opts.Force {
		params = append(params, "force=true")
	}

	return strings.Join(params, "&")
}

// handleErrorResponse converts HTTP error responses to Kubernetes API errors
func (pc *ProxyClient) handleErrorResponse(resp *http.Response, gvk schema.GroupVersionKind, name string) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as Kubernetes Status object
	var status metav1.Status
	if err := json.Unmarshal(body, &status); err == nil && status.Status == "Failure" {
		return &apierrors.StatusError{ErrStatus: status}
	}

	// Create appropriate error based on status code
	gr := schema.GroupResource{Group: gvk.Group, Resource: pluralizeKind(gvk.Kind)}
	errMsg := string(body)

	switch resp.StatusCode {
	case http.StatusNotFound:
		return apierrors.NewNotFound(gr, name)
	case http.StatusConflict:
		return apierrors.NewConflict(gr, name, fmt.Errorf("%s", errMsg))
	case http.StatusForbidden:
		return apierrors.NewForbidden(gr, name, fmt.Errorf("%s", errMsg))
	case http.StatusUnauthorized:
		return apierrors.NewUnauthorized(errMsg)
	case http.StatusBadRequest:
		return apierrors.NewBadRequest(errMsg)
	case http.StatusInternalServerError, http.StatusServiceUnavailable:
		return apierrors.NewInternalError(fmt.Errorf("%s", errMsg))
	default:
		return fmt.Errorf("proxy request failed with status %d: %s", resp.StatusCode, errMsg)
	}
}

// getGVKForList extracts GVK from a list object
func getGVKForList(list client.ObjectList, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvk, err := getGVK(list, scheme)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	// Convert list GVK to item GVK (e.g., PodList -> Pod)
	kind := strings.TrimSuffix(gvk.Kind, "List")
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Version: gvk.Version,
		Kind:    kind,
	}, nil
}

// pluralizeKind converts a Kind to its plural resource form
func pluralizeKind(kind string) string {
	// Simple pluralization - add 's' or 'es'
	// This works for most Kubernetes resources
	lower := strings.ToLower(kind)

	// Special cases
	switch lower {
	case "endpoints":
		return "endpoints"
	case "ingress":
		return "ingresses"
	case "networkpolicy":
		return "networkpolicies"
	}

	// General rules
	if strings.HasSuffix(lower, "s") || strings.HasSuffix(lower, "x") ||
		strings.HasSuffix(lower, "ch") || strings.HasSuffix(lower, "sh") {
		return lower + "es"
	}
	if strings.HasSuffix(lower, "y") && len(lower) > 1 {
		// Check if the letter before 'y' is a consonant
		beforeY := lower[len(lower)-2]
		if beforeY != 'a' && beforeY != 'e' && beforeY != 'i' && beforeY != 'o' && beforeY != 'u' {
			return lower[:len(lower)-1] + "ies"
		}
	}
	return lower + "s"
}

// isNamespaced returns true if the resource is namespaced
func isNamespaced(gvk schema.GroupVersionKind) bool {
	clusterScoped := map[string]bool{
		"Namespace":                      true,
		"Node":                           true,
		"PersistentVolume":               true,
		"ClusterRole":                    true,
		"ClusterRoleBinding":             true,
		"StorageClass":                   true,
		"CustomResourceDefinition":       true,
		"APIService":                     true,
		"ValidatingWebhookConfiguration": true,
		"MutatingWebhookConfiguration":   true,
		"PriorityClass":                  true,
	}

	return !clusterScoped[gvk.Kind]
}

// getGVK extracts GroupVersionKind from a runtime.Object using the scheme
func getGVK(obj runtime.Object, scheme *runtime.Scheme) (schema.GroupVersionKind, error) {
	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to get GVK: %w", err)
	}
	if len(gvks) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no GVK found for object")
	}
	return gvks[0], nil
}

// ProxyTLSConfig is defined in client.go to avoid import cycles
// We reference it here for type safety

// buildProxyTLSConfig builds a TLS config for the HTTP proxy client
func buildProxyTLSConfig(tlsConfig *ProxyTLSConfig) (*tls.Config, error) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// If no TLS config provided, fall back to insecure mode
	if tlsConfig == nil {
		cfg.InsecureSkipVerify = true
		return cfg, nil
	}

	// Load CA certificate for server verification
	if tlsConfig.CACertPath != "" {
		caCert, err := os.ReadFile(tlsConfig.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		cfg.RootCAs = caCertPool
	} else {
		// If no CA cert provided, use insecure mode
		cfg.InsecureSkipVerify = true
	}

	// Load client certificate and key for mTLS authentication
	if tlsConfig.ClientCertPath != "" && tlsConfig.ClientKeyPath != "" {
		clientCert, err := tls.LoadX509KeyPair(tlsConfig.ClientCertPath, tlsConfig.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate and key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{clientCert}
	}

	return cfg, nil
}
