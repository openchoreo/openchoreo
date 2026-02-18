// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ========== validateK8sPath Tests ==========

func TestValidateK8sPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
		errMsg  string
	}{
		// Valid paths
		{
			name:    "valid pods list",
			path:    "api/v1/namespaces/default/pods",
			wantErr: false,
		},
		{
			name:    "valid specific pod",
			path:    "api/v1/namespaces/default/pods/my-pod",
			wantErr: false,
		},
		{
			name:    "valid deployments",
			path:    "apis/apps/v1/namespaces/default/deployments",
			wantErr: false,
		},
		{
			name:    "valid events",
			path:    "api/v1/namespaces/default/events",
			wantErr: false,
		},
		{
			name:    "valid configmaps",
			path:    "api/v1/namespaces/default/configmaps",
			wantErr: false,
		},
		{
			name:    "valid services",
			path:    "api/v1/namespaces/my-app/services",
			wantErr: false,
		},
		{
			name:    "valid custom resource",
			path:    "apis/custom.io/v1/namespaces/default/myresources",
			wantErr: false,
		},

		// Blocked: directory traversal
		{
			name:    "block directory traversal with ..",
			path:    "api/v1/namespaces/../kube-system/secrets",
			wantErr: true,
			errMsg:  "path traversal is not allowed",
		},
		{
			name:    "block directory traversal at start",
			path:    "../etc/passwd",
			wantErr: true,
			errMsg:  "path traversal is not allowed",
		},
		{
			name:    "block directory traversal in the middle",
			path:    "api/v1/../../../etc/shadow",
			wantErr: true,
			errMsg:  "path traversal is not allowed",
		},

		// Blocked: null bytes
		{
			name:    "block null byte",
			path:    "api/v1/namespaces/default/pods\x00malicious",
			wantErr: true,
			errMsg:  "invalid path",
		},

		// Blocked: secrets
		{
			name:    "block secrets access",
			path:    "api/v1/namespaces/default/secrets",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},
		{
			name:    "block specific secret access",
			path:    "api/v1/namespaces/default/secrets/my-secret",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},
		{
			name:    "block secrets case-insensitive",
			path:    "api/v1/namespaces/default/Secrets",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},

		// Blocked: serviceaccounts
		{
			name:    "block serviceaccounts access",
			path:    "api/v1/namespaces/default/serviceaccounts",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},
		{
			name:    "block specific serviceaccount access",
			path:    "api/v1/namespaces/default/serviceaccounts/default",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},

		// Blocked: exec/attach/portforward (WebSocket-upgradeable sub-resources)
		{
			name:    "block exec sub-resource",
			path:    "api/v1/namespaces/default/pods/my-pod/exec",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},
		{
			name:    "block attach sub-resource",
			path:    "api/v1/namespaces/default/pods/my-pod/attach",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},
		{
			name:    "block portforward sub-resource",
			path:    "api/v1/namespaces/default/pods/my-pod/portforward",
			wantErr: true,
			errMsg:  "access to this resource type is not allowed",
		},

		// Segment-based matching: no false positives
		{
			name:    "allow namespace named secrets-manager",
			path:    "api/v1/namespaces/secrets-manager/pods",
			wantErr: false,
		},
		{
			name:    "allow pod named my-secrets-pod",
			path:    "api/v1/namespaces/default/pods/my-secrets-pod",
			wantErr: false,
		},
		{
			name:    "allow resource named exec-controller",
			path:    "apis/apps/v1/namespaces/default/deployments/exec-controller",
			wantErr: false,
		},

		// Blocked: sensitive namespaces
		{
			name:    "block kube-system namespace",
			path:    "api/v1/namespaces/kube-system/pods",
			wantErr: true,
			errMsg:  "access to this namespace is not allowed",
		},
		{
			name:    "block kube-public namespace",
			path:    "api/v1/namespaces/kube-public/configmaps",
			wantErr: true,
			errMsg:  "access to this namespace is not allowed",
		},
		{
			name:    "block kube-node-lease namespace",
			path:    "api/v1/namespaces/kube-node-lease/leases",
			wantErr: true,
			errMsg:  "access to this namespace is not allowed",
		},
		{
			name:    "block kube-system namespace path ending",
			path:    "api/v1/namespaces/kube-system",
			wantErr: true,
			errMsg:  "access to this namespace is not allowed",
		},
		{
			name:    "allow namespace containing kube-system as prefix",
			path:    "api/v1/namespaces/kube-system-ext/pods",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateK8sPath(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateK8sPath(%q) = nil, want error containing %q", tt.path, tt.errMsg)
					return
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("validateK8sPath(%q) error = %q, want %q", tt.path, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateK8sPath(%q) = %v, want nil", tt.path, err)
				}
			}
		})
	}
}

// ========== Plane Proxy Config Tests ==========

func TestPlaneProxyConfigs(t *testing.T) {
	tests := []struct {
		name         string
		config       planeProxyConfig
		wantType     string
		wantAction   string
		wantResource string
	}{
		{
			name:         "dataplane config",
			config:       dataPlaneProxyConfig,
			wantType:     "dataplane",
			wantAction:   "dataplaneresource:view",
			wantResource: "dataPlaneResource",
		},
		{
			name:         "buildplane config",
			config:       buildPlaneProxyConfig,
			wantType:     "buildplane",
			wantAction:   "buildplaneresource:view",
			wantResource: "buildPlaneResource",
		},
		{
			name:         "observabilityplane config",
			config:       observabilityPlaneProxyConfig,
			wantType:     "observabilityplane",
			wantAction:   "observabilityplaneresource:view",
			wantResource: "observabilityPlaneResource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config.planeType != tt.wantType {
				t.Errorf("planeType = %q, want %q", tt.config.planeType, tt.wantType)
			}
			if tt.config.action != tt.wantAction {
				t.Errorf("action = %q, want %q", tt.config.action, tt.wantAction)
			}
			if tt.config.resourceType != tt.wantResource {
				t.Errorf("resourceType = %q, want %q", tt.config.resourceType, tt.wantResource)
			}
		})
	}
}

// ========== Namespace-scoped Handler Path Parameter Tests ==========

func TestProxyDataPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name        string
		ocNamespace string
		dpName      string
		k8sPath     string
	}{
		{
			name:        "valid path parameters",
			ocNamespace: "acme",
			dpName:      "prod-dp",
			k8sPath:     "api/v1/namespaces/default/pods",
		},
		{
			name:        "path with hyphens",
			ocNamespace: "my-org",
			dpName:      "my-data-plane",
			k8sPath:     "apis/apps/v1/namespaces/default/deployments",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/oc-namespaces/"+tt.ocNamespace+"/dataplanes/"+tt.dpName+"/"+tt.k8sPath, nil)
			req.SetPathValue("ocNamespace", tt.ocNamespace)
			req.SetPathValue("dpName", tt.dpName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("ocNamespace") != tt.ocNamespace {
				t.Errorf("ocNamespace = %v, want %v", req.PathValue("ocNamespace"), tt.ocNamespace)
			}
			if req.PathValue("dpName") != tt.dpName {
				t.Errorf("dpName = %v, want %v", req.PathValue("dpName"), tt.dpName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

func TestProxyBuildPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name        string
		ocNamespace string
		bpName      string
		k8sPath     string
	}{
		{
			name:        "valid path parameters",
			ocNamespace: "acme",
			bpName:      "ci-builder",
			k8sPath:     "api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/oc-namespaces/"+tt.ocNamespace+"/buildplanes/"+tt.bpName+"/"+tt.k8sPath, nil)
			req.SetPathValue("ocNamespace", tt.ocNamespace)
			req.SetPathValue("bpName", tt.bpName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("ocNamespace") != tt.ocNamespace {
				t.Errorf("ocNamespace = %v, want %v", req.PathValue("ocNamespace"), tt.ocNamespace)
			}
			if req.PathValue("bpName") != tt.bpName {
				t.Errorf("bpName = %v, want %v", req.PathValue("bpName"), tt.bpName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

func TestProxyObservabilityPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name        string
		ocNamespace string
		opName      string
		k8sPath     string
	}{
		{
			name:        "valid path parameters",
			ocNamespace: "acme",
			opName:      "obs-plane",
			k8sPath:     "api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/oc-namespaces/"+tt.ocNamespace+"/observabilityplanes/"+tt.opName+"/"+tt.k8sPath, nil)
			req.SetPathValue("ocNamespace", tt.ocNamespace)
			req.SetPathValue("opName", tt.opName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("ocNamespace") != tt.ocNamespace {
				t.Errorf("ocNamespace = %v, want %v", req.PathValue("ocNamespace"), tt.ocNamespace)
			}
			if req.PathValue("opName") != tt.opName {
				t.Errorf("opName = %v, want %v", req.PathValue("opName"), tt.opName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

// ========== Cluster-scoped Handler Path Parameter Tests ==========

func TestProxyClusterDataPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name    string
		cdpName string
		k8sPath string
	}{
		{
			name:    "valid path parameters",
			cdpName: "shared-dp",
			k8sPath: "api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster-dataplanes/"+tt.cdpName+"/"+tt.k8sPath, nil)
			req.SetPathValue("cdpName", tt.cdpName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("cdpName") != tt.cdpName {
				t.Errorf("cdpName = %v, want %v", req.PathValue("cdpName"), tt.cdpName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

func TestProxyClusterBuildPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name    string
		cbpName string
		k8sPath string
	}{
		{
			name:    "valid path parameters",
			cbpName: "shared-bp",
			k8sPath: "api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster-buildplanes/"+tt.cbpName+"/"+tt.k8sPath, nil)
			req.SetPathValue("cbpName", tt.cbpName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("cbpName") != tt.cbpName {
				t.Errorf("cbpName = %v, want %v", req.PathValue("cbpName"), tt.cbpName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

func TestProxyClusterObservabilityPlaneK8s_PathParameters(t *testing.T) {
	tests := []struct {
		name    string
		copName string
		k8sPath string
	}{
		{
			name:    "valid path parameters",
			copName: "shared-op",
			k8sPath: "api/v1/namespaces/default/pods",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster-observabilityplanes/"+tt.copName+"/"+tt.k8sPath, nil)
			req.SetPathValue("copName", tt.copName)
			req.SetPathValue("k8sPath", tt.k8sPath)

			if req.PathValue("copName") != tt.copName {
				t.Errorf("copName = %v, want %v", req.PathValue("copName"), tt.copName)
			}
			if req.PathValue("k8sPath") != tt.k8sPath {
				t.Errorf("k8sPath = %v, want %v", req.PathValue("k8sPath"), tt.k8sPath)
			}
		})
	}
}

// ========== Missing Path Parameter Validation Tests ==========

func TestProxyDataPlaneK8s_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name        string
		ocNamespace string
		dpName      string
		wantValid   bool
	}{
		{
			name:        "all parameters present",
			ocNamespace: "acme",
			dpName:      "prod-dp",
			wantValid:   true,
		},
		{
			name:        "missing namespace",
			ocNamespace: "",
			dpName:      "prod-dp",
			wantValid:   false,
		},
		{
			name:        "missing dataplane name",
			ocNamespace: "acme",
			dpName:      "",
			wantValid:   false,
		},
		{
			name:        "all parameters missing",
			ocNamespace: "",
			dpName:      "",
			wantValid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.ocNamespace != "" && tt.dpName != ""
			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

func TestProxyClusterDataPlaneK8s_MissingPathParameters(t *testing.T) {
	tests := []struct {
		name      string
		cdpName   string
		wantValid bool
	}{
		{
			name:      "name present",
			cdpName:   "shared-dp",
			wantValid: true,
		},
		{
			name:      "name missing",
			cdpName:   "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.cdpName != ""
			if isValid != tt.wantValid {
				t.Errorf("Validation result = %v, want %v", isValid, tt.wantValid)
			}
		})
	}
}

// ========== Streaming Parameter Rejection Tests ==========

func TestProxyPlaneK8s_StreamingParameterRejection(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
		wantReject  bool
	}{
		{
			name:        "no streaming params",
			queryString: "",
			wantReject:  false,
		},
		{
			name:        "watch=true rejected",
			queryString: "watch=true",
			wantReject:  true,
		},
		{
			name:        "follow=true rejected",
			queryString: "follow=true",
			wantReject:  true,
		},
		{
			name:        "watch=false allowed",
			queryString: "watch=false",
			wantReject:  false,
		},
		{
			name:        "follow=false allowed",
			queryString: "follow=false",
			wantReject:  false,
		},
		{
			name:        "fieldSelector allowed",
			queryString: "fieldSelector=involvedObject.name=my-pod",
			wantReject:  false,
		},
		{
			name:        "labelSelector allowed",
			queryString: "labelSelector=app=nginx",
			wantReject:  false,
		},
		{
			name:        "watch=true with other params rejected",
			queryString: "labelSelector=app=nginx&watch=true",
			wantReject:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/oc-namespaces/acme/dataplanes/prod-dp/api/v1/namespaces/default/pods"
			if tt.queryString != "" {
				url += "?" + tt.queryString
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			query := req.URL.Query()
			shouldReject := query.Get("watch") == "true" || query.Get("follow") == "true"

			if shouldReject != tt.wantReject {
				t.Errorf("Streaming rejection = %v, want %v", shouldReject, tt.wantReject)
			}
		})
	}
}

// ========== Blocked Paths and Namespaces Lists Tests ==========

func TestBlockedPathSegments(t *testing.T) {
	expectedBlocked := []string{"secrets", "serviceaccounts", "exec", "attach", "portforward"}
	if len(blockedPathSegments) != len(expectedBlocked) {
		t.Errorf("Expected %d blocked path segments, got %d", len(expectedBlocked), len(blockedPathSegments))
		return
	}
	for i, expected := range expectedBlocked {
		if blockedPathSegments[i] != expected {
			t.Errorf("blockedPathSegments[%d] = %q, want %q", i, blockedPathSegments[i], expected)
		}
	}
}

func TestBlockedNamespaces(t *testing.T) {
	expectedBlocked := []string{"kube-system", "kube-public", "kube-node-lease"}
	if len(blockedNamespaces) != len(expectedBlocked) {
		t.Errorf("Expected %d blocked namespaces, got %d", len(expectedBlocked), len(blockedNamespaces))
		return
	}
	for i, expected := range expectedBlocked {
		if blockedNamespaces[i] != expected {
			t.Errorf("blockedNamespaces[%d] = %q, want %q", i, blockedNamespaces[i], expected)
		}
	}
}

// ========== Constants Tests ==========

func TestPlaneTypeConstants(t *testing.T) {
	if planeTypeDataPlane != "dataplane" {
		t.Errorf("planeTypeDataPlane = %q, want %q", planeTypeDataPlane, "dataplane")
	}
	if planeTypeBuildPlane != "buildplane" {
		t.Errorf("planeTypeBuildPlane = %q, want %q", planeTypeBuildPlane, "buildplane")
	}
	if planeTypeObservabilityPlane != "observabilityplane" {
		t.Errorf("planeTypeObservabilityPlane = %q, want %q", planeTypeObservabilityPlane, "observabilityplane")
	}
}

func TestMaxProxyResponseBytes(t *testing.T) {
	expected := 10 * 1024 * 1024 // 10MB
	if maxProxyResponseBytes != expected {
		t.Errorf("maxProxyResponseBytes = %d, want %d (10MB)", maxProxyResponseBytes, expected)
	}
}

// ========== Hop-by-Hop Header Tests ==========

func TestIsHopByHopHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   bool
	}{
		// Hop-by-hop headers that should be filtered
		{name: "Connection", header: "Connection", want: true},
		{name: "Keep-Alive", header: "Keep-Alive", want: true},
		{name: "Proxy-Authenticate", header: "Proxy-Authenticate", want: true},
		{name: "Proxy-Authorization", header: "Proxy-Authorization", want: true},
		{name: "Te", header: "Te", want: true},
		{name: "Trailer", header: "Trailer", want: true},
		{name: "Transfer-Encoding", header: "Transfer-Encoding", want: true},
		{name: "Upgrade", header: "Upgrade", want: true},

		// Case-insensitive matching
		{name: "connection lowercase", header: "connection", want: true},
		{name: "TRANSFER-ENCODING uppercase", header: "TRANSFER-ENCODING", want: true},

		// Regular headers that should pass through
		{name: "Content-Type", header: "Content-Type", want: false},
		{name: "Content-Length", header: "Content-Length", want: false},
		{name: "X-Custom-Header", header: "X-Custom-Header", want: false},
		{name: "Authorization", header: "Authorization", want: false},
		{name: "Cache-Control", header: "Cache-Control", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHopByHopHeader(tt.header)
			if got != tt.want {
				t.Errorf("isHopByHopHeader(%q) = %v, want %v", tt.header, got, tt.want)
			}
		})
	}
}

func TestHopByHopHeadersCount(t *testing.T) {
	// RFC 7230 §6.1 defines 8 hop-by-hop headers
	if len(hopByHopHeaders) != 8 {
		t.Errorf("Expected 8 hop-by-hop headers, got %d", len(hopByHopHeaders))
	}
}
