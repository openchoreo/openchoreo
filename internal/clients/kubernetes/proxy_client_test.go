// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ───────────────────────── Test helpers ─────────────────────────

func newTestProxyClient(t *testing.T, handler http.HandlerFunc) *ProxyClient {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return &ProxyClient{
		gatewayURL:  server.URL,
		planeType:   "dataplane",
		planeID:     "test-plane",
		crNamespace: "test-ns",
		crName:      "test-cr",
		httpClient:  server.Client(),
		scheme:      k8sscheme.Scheme,
	}
}

func fakeResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func boolPtr(b bool) *bool { return &b }

type unknownRuntimeObject struct {
	metav1.TypeMeta
}

func (o *unknownRuntimeObject) DeepCopyObject() runtime.Object {
	if o == nil {
		return nil
	}
	copy := *o
	return &copy
}

const (
	testPodName      = "nginx"
	testPodNamespace = "default"

	testPodJSON     = `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"nginx","namespace":"default"}}`
	testPodListJSON = `{"apiVersion":"v1","kind":"PodList","metadata":{},"items":[{"metadata":{"name":"nginx","namespace":"default"}}]}`
)

// ───────────────────────── Pure-logic helper tests ─────────────────────────

func TestNewProxyClientValidation(t *testing.T) {
	tests := []struct {
		name            string
		gatewayURL      string
		planeIdentifier string
		crNamespace     string
		crName          string
		tlsConfig       *ProxyTLSConfig
		wantErr         string
	}{
		{
			name:            "empty gateway URL",
			gatewayURL:      "",
			planeIdentifier: "dataplane/test-plane",
			crNamespace:     "test-ns",
			crName:          "test-cr",
			wantErr:         "gatewayURL is required",
		},
		{
			name:            "empty plane identifier",
			gatewayURL:      "https://gateway.example.com",
			planeIdentifier: "",
			crNamespace:     "test-ns",
			crName:          "test-cr",
			wantErr:         "planeIdentifier is required",
		},
		{
			name:            "empty CR name",
			gatewayURL:      "https://gateway.example.com",
			planeIdentifier: "dataplane/test-plane",
			crNamespace:     "test-ns",
			crName:          "",
			wantErr:         "crName is required",
		},
		{
			name:            "invalid plane identifier format",
			gatewayURL:      "https://gateway.example.com",
			planeIdentifier: "dataplane-only",
			crNamespace:     "test-ns",
			crName:          "test-cr",
			wantErr:         "invalid planeIdentifier format",
		},
		{
			name:            "tls config build failure",
			gatewayURL:      "https://gateway.example.com",
			planeIdentifier: "dataplane/test-plane",
			crNamespace:     "test-ns",
			crName:          "test-cr",
			tlsConfig: &ProxyTLSConfig{
				CACertPath: "/path/does/not/exist/ca.crt",
			},
			wantErr: "failed to build TLS config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProxyClient(tt.gatewayURL, tt.planeIdentifier, tt.crNamespace, tt.crName, tt.tlsConfig)
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestPluralizeKind(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		// Special-cased values
		{"Endpoints", "endpoints"},
		{"Ingress", "ingresses"},
		{"NetworkPolicy", "networkpolicies"},
		// Ends with 's' → add 'es'
		{"Class", "classes"},
		// Ends with 'x' → add 'es'
		{"Prefix", "prefixes"},
		// Ends with 'sh' → add 'es'
		{"Mesh", "meshes"},
		// Ends with 'ch' → add 'es'
		{"Patch", "patches"},
		// Ends with consonant-y → change to 'ies'
		{"StoragePolicy", "storagepolicies"},
		{"Canary", "canaries"},
		// Vowel-y → just add 's'
		{"Key", "keys"},
		// Default: just add 's'
		{"Pod", "pods"},
		{"Service", "services"},
		{"Deployment", "deployments"},
		{"ConfigMap", "configmaps"},
		{"Namespace", "namespaces"},
		{"Node", "nodes"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			assert.Equal(t, tt.want, pluralizeKind(tt.kind))
		})
	}
}

func TestIsNamespaced(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		// Cluster-scoped resources
		{"Namespace", false},
		{"Node", false},
		{"PersistentVolume", false},
		{"ClusterRole", false},
		{"ClusterRoleBinding", false},
		{"StorageClass", false},
		{"CustomResourceDefinition", false},
		{"APIService", false},
		{"ValidatingWebhookConfiguration", false},
		{"MutatingWebhookConfiguration", false},
		{"PriorityClass", false},
		// Namespaced resources (not in cluster-scoped map)
		{"Pod", true},
		{"Service", true},
		{"Deployment", true},
		{"ConfigMap", true},
		{"Secret", true},
		{"ServiceAccount", true},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			gvk := schema.GroupVersionKind{Kind: tt.kind}
			assert.Equal(t, tt.want, isNamespaced(gvk))
		})
	}
}

func TestGetGVK(t *testing.T) {
	// Pod is in the core group (empty string)
	pod := &corev1.Pod{}
	gvk, err := getGVK(pod, k8sscheme.Scheme)
	require.NoError(t, err)
	assert.Equal(t, "", gvk.Group)
	assert.Equal(t, "v1", gvk.Version)
	assert.Equal(t, "Pod", gvk.Kind)

	// ConfigMap is also core
	cm := &corev1.ConfigMap{}
	gvk, err = getGVK(cm, k8sscheme.Scheme)
	require.NoError(t, err)
	assert.Equal(t, "ConfigMap", gvk.Kind)
	assert.Equal(t, "v1", gvk.Version)

	// Unknown type should fail scheme lookup
	_, err = getGVK(&unknownRuntimeObject{}, k8sscheme.Scheme)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get GVK")
}

func TestBuildProxyTLSConfig(t *testing.T) {
	t.Run("nil config uses default verification", func(t *testing.T) {
		cfg, err := buildProxyTLSConfig(nil)
		require.NoError(t, err)
		assert.False(t, cfg.InsecureSkipVerify)
		assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	})

	t.Run("empty config without CA uses default verification", func(t *testing.T) {
		cfg, err := buildProxyTLSConfig(&ProxyTLSConfig{})
		require.NoError(t, err)
		assert.False(t, cfg.InsecureSkipVerify)
		assert.Nil(t, cfg.RootCAs)
	})

	t.Run("Insecure=true opts into skipping verification", func(t *testing.T) {
		cfg, err := buildProxyTLSConfig(&ProxyTLSConfig{Insecure: true})
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
	})

	t.Run("Insecure=true short-circuits before reading CA", func(t *testing.T) {
		// Confirms that a misconfigured CA path is ignored when the caller has
		// explicitly opted into insecure mode — no surprise read errors.
		cfg, err := buildProxyTLSConfig(&ProxyTLSConfig{
			Insecure:   true,
			CACertPath: "/path/does/not/exist/ca.crt",
		})
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
		assert.Nil(t, cfg.RootCAs)
	})

	t.Run("Insecure=true still loads client cert for mTLS", func(t *testing.T) {
		certPEM, keyPEM := mustCreateSelfSignedCertAndKeyPEM(t)
		dir := t.TempDir()
		certFile := dir + "/client.crt"
		keyFile := dir + "/client.key"
		require.NoError(t, os.WriteFile(certFile, certPEM, 0o600))
		require.NoError(t, os.WriteFile(keyFile, keyPEM, 0o600))

		cfg, err := buildProxyTLSConfig(&ProxyTLSConfig{
			Insecure:       true,
			ClientCertPath: certFile,
			ClientKeyPath:  keyFile,
		})
		require.NoError(t, err)
		assert.True(t, cfg.InsecureSkipVerify)
		assert.Len(t, cfg.Certificates, 1)
	})

	t.Run("only ClientCertPath set returns asymmetric-config error", func(t *testing.T) {
		_, err := buildProxyTLSConfig(&ProxyTLSConfig{ClientCertPath: "/tmp/client.crt"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both ClientCertPath and ClientKeyPath must be set")
	})

	t.Run("only ClientKeyPath set returns asymmetric-config error", func(t *testing.T) {
		_, err := buildProxyTLSConfig(&ProxyTLSConfig{ClientKeyPath: "/tmp/client.key"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both ClientCertPath and ClientKeyPath must be set")
	})

	t.Run("invalid CA path returns error", func(t *testing.T) {
		_, err := buildProxyTLSConfig(&ProxyTLSConfig{CACertPath: "/path/does/not/exist/ca.crt"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read CA certificate")
	})

	t.Run("invalid CA pem returns parse error", func(t *testing.T) {
		badCAFile := t.TempDir() + "/bad-ca.crt"
		err := os.WriteFile(badCAFile, []byte("not-a-cert"), 0o600)
		require.NoError(t, err)

		_, err = buildProxyTLSConfig(&ProxyTLSConfig{CACertPath: badCAFile})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse CA certificate")
	})

	t.Run("valid CA and invalid mTLS pair returns keypair error", func(t *testing.T) {
		caCertPEM := mustCreateSelfSignedCertPEM(t)
		caFile := t.TempDir() + "/ca.crt"
		err := os.WriteFile(caFile, caCertPEM, 0o600)
		require.NoError(t, err)

		_, err = buildProxyTLSConfig(&ProxyTLSConfig{
			CACertPath:     caFile,
			ClientCertPath: "/missing/client.crt",
			ClientKeyPath:  "/missing/client.key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load client certificate and key")
	})
}

func mustCreateSelfSignedCertPEM(t *testing.T) []byte {
	certPEM, _ := mustCreateSelfSignedCertAndKeyPEM(t)
	return certPEM
}

func mustCreateSelfSignedCertAndKeyPEM(t *testing.T) ([]byte, []byte) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}

func TestGetGVKForList(t *testing.T) {
	// PodList → Pod
	podList := &corev1.PodList{}
	gvk, err := getGVKForList(podList, k8sscheme.Scheme)
	require.NoError(t, err)
	assert.Equal(t, "Pod", gvk.Kind)
	assert.Equal(t, "v1", gvk.Version)
	assert.Equal(t, "", gvk.Group)

	// ConfigMapList → ConfigMap
	cmList := &corev1.ConfigMapList{}
	gvk, err = getGVKForList(cmList, k8sscheme.Scheme)
	require.NoError(t, err)
	assert.Equal(t, "ConfigMap", gvk.Kind)
}

func TestBuildGetPath(t *testing.T) {
	pc := &ProxyClient{}

	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		namespace string
		resName   string
		want      string
	}{
		{
			name:      "core namespaced",
			gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			namespace: testPodNamespace,
			resName:   testPodName,
			want:      "/api/v1/namespaces/default/pods/nginx",
		},
		{
			name:      "core cluster-scoped",
			gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
			namespace: "",
			resName:   "kube-system",
			want:      "/api/v1/namespaces/kube-system",
		},
		{
			name:      "core cluster-scoped with namespace arg ignored",
			gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Node"},
			namespace: testPodNamespace, // should be ignored since Node is cluster-scoped
			resName:   "node-1",
			want:      "/api/v1/nodes/node-1",
		},
		{
			name:      "non-core namespaced",
			gvk:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			namespace: testPodNamespace,
			resName:   "web",
			want:      "/apis/apps/v1/namespaces/default/deployments/web",
		},
		{
			name:      "non-core cluster-scoped",
			gvk:       schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
			namespace: "",
			resName:   "admin",
			want:      "/apis/rbac.authorization.k8s.io/v1/clusterroles/admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pc.buildGetPath(tt.gvk, tt.namespace, tt.resName))
		})
	}
}

func TestBuildListPath(t *testing.T) {
	pc := &ProxyClient{}

	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		namespace string
		want      string
	}{
		{
			name:      "core namespaced",
			gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"},
			namespace: testPodNamespace,
			want:      "/api/v1/namespaces/default/pods",
		},
		{
			name:      "core cluster-scoped",
			gvk:       schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"},
			namespace: "",
			want:      "/api/v1/namespaces",
		},
		{
			name:      "non-core namespaced",
			gvk:       schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			namespace: "ns1",
			want:      "/apis/apps/v1/namespaces/ns1/deployments",
		},
		{
			name:      "non-core cluster-scoped",
			gvk:       schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"},
			namespace: "",
			want:      "/apis/rbac.authorization.k8s.io/v1/clusterroles",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, pc.buildListPath(tt.gvk, tt.namespace))
		})
	}
}

func TestBuildStatusPath(t *testing.T) {
	pc := &ProxyClient{}
	gvk := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}

	got := pc.buildStatusPath(gvk, testPodNamespace, "web")
	assert.Equal(t, "/apis/apps/v1/namespaces/default/deployments/web/status", got)
}

// TestBuildPathDelegates verifies that buildCreatePath delegates to buildListPath,
// and that buildUpdatePath and buildDeletePath both delegate to buildGetPath.
func TestBuildPathDelegates(t *testing.T) {
	pc := &ProxyClient{}
	gvk := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}

	assert.Equal(t, pc.buildListPath(gvk, testPodNamespace), pc.buildCreatePath(gvk, testPodNamespace), "buildCreatePath should delegate to buildListPath")
	assert.Equal(t, pc.buildGetPath(gvk, testPodNamespace, testPodName), pc.buildUpdatePath(gvk, testPodNamespace, testPodName), "buildUpdatePath should delegate to buildGetPath")
	assert.Equal(t, pc.buildGetPath(gvk, testPodNamespace, testPodName), pc.buildDeletePath(gvk, testPodNamespace, testPodName), "buildDeletePath should delegate to buildGetPath")
}

func TestBuildProxyURL(t *testing.T) {
	tests := []struct {
		name        string
		gatewayURL  string
		planeType   string
		planeID     string
		crNamespace string
		crName      string
		apiPath     string
		want        string
	}{
		{
			name:        "standard namespaced",
			gatewayURL:  "https://gateway.example.com",
			planeType:   "dataplane",
			planeID:     "prod",
			crNamespace: "acme",
			crName:      "dp1",
			apiPath:     "/api/v1/pods",
			want:        "https://gateway.example.com/api/proxy/dataplane/prod/acme/dp1/k8s/api/v1/pods",
		},
		{
			name:        "cluster-scoped with _cluster namespace",
			gatewayURL:  "https://gw",
			planeType:   "dataplane",
			planeID:     "shared",
			crNamespace: "_cluster",
			crName:      "shared-dp",
			apiPath:     "/api/v1/nodes",
			want:        "https://gw/api/proxy/dataplane/shared/_cluster/shared-dp/k8s/api/v1/nodes",
		},
		{
			name:        "workflowplane",
			gatewayURL:  "https://gw",
			planeType:   "workflowplane",
			planeID:     "ci",
			crNamespace: "team-a",
			crName:      "ci-wp",
			apiPath:     "/apis/argoproj.io/v1alpha1/workflows",
			want:        "https://gw/api/proxy/workflowplane/ci/team-a/ci-wp/k8s/apis/argoproj.io/v1alpha1/workflows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &ProxyClient{
				gatewayURL:  tt.gatewayURL,
				planeType:   tt.planeType,
				planeID:     tt.planeID,
				crNamespace: tt.crNamespace,
				crName:      tt.crName,
			}
			assert.Equal(t, tt.want, pc.buildProxyURL(tt.apiPath))
		})
	}
}

func TestBuildListQueryParams(t *testing.T) {
	pc := &ProxyClient{}

	t.Run("empty options", func(t *testing.T) {
		opts := &client.ListOptions{}
		assert.Empty(t, pc.buildListQueryParams(opts))
	})

	t.Run("label selector only", func(t *testing.T) {
		opts := &client.ListOptions{}
		client.MatchingLabels{"app": "web"}.ApplyToList(opts)
		got := pc.buildListQueryParams(opts)
		assert.Contains(t, got, "labelSelector=")
		assert.Contains(t, got, "app")
	})

	t.Run("limit only", func(t *testing.T) {
		opts := &client.ListOptions{Limit: 100}
		assert.Equal(t, "limit=100", pc.buildListQueryParams(opts))
	})

	t.Run("continue token only", func(t *testing.T) {
		opts := &client.ListOptions{Continue: "abc123"}
		assert.Equal(t, "continue=abc123", pc.buildListQueryParams(opts))
	})

	t.Run("limit and continue", func(t *testing.T) {
		opts := &client.ListOptions{Limit: 50, Continue: "tok"}
		got := pc.buildListQueryParams(opts)
		assert.Contains(t, got, "limit=50")
		assert.Contains(t, got, "continue=tok")
	})
}

func TestBuildPatchQueryParams(t *testing.T) {
	pc := &ProxyClient{}

	tests := []struct {
		name         string
		opts         *client.PatchOptions
		wantContains []string
		wantEmpty    bool
	}{
		{
			name:      "empty opts",
			opts:      &client.PatchOptions{},
			wantEmpty: true,
		},
		{
			name:         "field manager only",
			opts:         &client.PatchOptions{FieldManager: "my-controller"},
			wantContains: []string{"fieldManager=my-controller"},
		},
		{
			name:         "force true",
			opts:         &client.PatchOptions{Force: boolPtr(true)},
			wantContains: []string{"force=true"},
		},
		{
			name:      "force false",
			opts:      &client.PatchOptions{Force: boolPtr(false)},
			wantEmpty: true,
		},
		{
			name:         "field manager and force",
			opts:         &client.PatchOptions{FieldManager: "mgr", Force: boolPtr(true)},
			wantContains: []string{"fieldManager=mgr", "force=true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pc.buildPatchQueryParams(tt.opts)
			if tt.wantEmpty {
				assert.Empty(t, got)
				return
			}
			for _, want := range tt.wantContains {
				assert.Contains(t, got, want)
			}
		})
	}
}

func TestBuildSubResourcePatchQueryParams(t *testing.T) {
	pc := &ProxyClient{}

	t.Run("empty", func(t *testing.T) {
		opts := &client.SubResourcePatchOptions{}
		assert.Empty(t, pc.buildSubResourcePatchQueryParams(opts))
	})

	t.Run("field manager and force", func(t *testing.T) {
		opts := &client.SubResourcePatchOptions{}
		opts.FieldManager = "test-mgr"
		opts.Force = boolPtr(true)
		got := pc.buildSubResourcePatchQueryParams(opts)
		assert.Contains(t, got, "fieldManager=test-mgr")
		assert.Contains(t, got, "force=true")
	})
}

func TestHandleErrorResponse(t *testing.T) {
	pc := &ProxyClient{}
	podGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}

	tests := []struct {
		name       string
		statusCode int
		body       string
		errCheck   func(error) bool
		wantMsg    string
	}{
		{
			name:       "kubernetes Status object → StatusError",
			statusCode: http.StatusNotFound,
			body:       `{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","message":"pods \"nginx\" not found","code":404}`,
			errCheck:   apierrors.IsNotFound,
		},
		{
			name:       "plain 404 body → NotFound",
			statusCode: http.StatusNotFound,
			body:       "pod not found",
			errCheck:   apierrors.IsNotFound,
		},
		{
			name:       "409 conflict",
			statusCode: http.StatusConflict,
			body:       "already exists",
			errCheck:   apierrors.IsConflict,
		},
		{
			name:       "403 forbidden",
			statusCode: http.StatusForbidden,
			body:       "forbidden",
			errCheck:   apierrors.IsForbidden,
		},
		{
			name:       "401 unauthorized",
			statusCode: http.StatusUnauthorized,
			body:       "unauthorized",
			errCheck:   apierrors.IsUnauthorized,
		},
		{
			name:       "400 bad request",
			statusCode: http.StatusBadRequest,
			body:       "bad request",
			errCheck:   apierrors.IsBadRequest,
		},
		{
			name:       "500 internal error",
			statusCode: http.StatusInternalServerError,
			body:       "internal error",
			errCheck:   apierrors.IsInternalError,
		},
		{
			name:       "503 service unavailable → internal error",
			statusCode: http.StatusServiceUnavailable,
			body:       "unavailable",
			errCheck:   apierrors.IsInternalError,
		},
		{
			name:       "unknown 418 → generic error with status code",
			statusCode: 418,
			body:       "teapot",
			wantMsg:    "418",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := fakeResponse(tt.statusCode, tt.body)
			err := pc.handleErrorResponse(resp, podGVK, testPodName)

			require.Error(t, err)
			if tt.errCheck != nil {
				assert.True(t, tt.errCheck(err), "errCheck failed for error: %v (type %T)", err, err)
			}
			if tt.wantMsg != "" {
				assert.Contains(t, err.Error(), tt.wantMsg)
			}
		})
	}
}

// ───────────────────────── ProxyClient HTTP CRUD tests ─────────────────────────

func TestProxyClientGet(t *testing.T) {
	t.Run("success returns populated object", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedAccept string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedAccept = r.Header.Get("Accept")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		err := pc.Get(context.Background(), client.ObjectKey{Namespace: testPodNamespace, Name: testPodName}, pod)

		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, capturedMethod)
		assert.True(t, strings.HasSuffix(capturedPath, "/pods/nginx"), "path %q should end with /pods/nginx", capturedPath)
		assert.Equal(t, "application/json", capturedAccept)
		assert.Equal(t, testPodName, pod.Name)
	})

	t.Run("404 returns NotFound error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"NotFound","code":404}`))
		})

		pod := &corev1.Pod{}
		err := pc.Get(context.Background(), client.ObjectKey{Namespace: testPodNamespace, Name: testPodName}, pod)

		require.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err), "expected NotFound error, got %T: %v", err, err)
	})

	t.Run("500 returns error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		})

		pod := &corev1.Pod{}
		err := pc.Get(context.Background(), client.ObjectKey{Namespace: testPodNamespace, Name: testPodName}, pod)

		require.Error(t, err)
	})
}

func TestProxyClientList(t *testing.T) {
	t.Run("success returns list", func(t *testing.T) {
		var capturedPath string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodListJSON))
		})

		podList := &corev1.PodList{}
		err := pc.List(context.Background(), podList)

		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(capturedPath, "/pods"), "list path %q should end with /pods", capturedPath)
		assert.Len(t, podList.Items, 1)
	})

	t.Run("with label selector adds query param", func(t *testing.T) {
		var capturedQuery string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodListJSON))
		})

		podList := &corev1.PodList{}
		err := pc.List(context.Background(), podList, client.MatchingLabels{"app": "web"})

		require.NoError(t, err)
		assert.Contains(t, capturedQuery, "labelSelector=")
	})

	t.Run("with namespace sets namespaced path", func(t *testing.T) {
		var capturedPath string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodListJSON))
		})

		podList := &corev1.PodList{}
		err := pc.List(context.Background(), podList, client.InNamespace("my-ns"))

		require.NoError(t, err)
		assert.Contains(t, capturedPath, "namespaces/my-ns")
	})

	t.Run("500 returns error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		err := pc.List(context.Background(), &corev1.PodList{})
		require.Error(t, err)
	})
}

func TestProxyClientCreate(t *testing.T) {
	t.Run("success updates object from response", func(t *testing.T) {
		var capturedMethod, capturedContentType string
		var capturedPath string
		const serverAssignedUID = "server-assigned-uid-abc123"
		serverResponse := `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"nginx","namespace":"default","uid":"` + serverAssignedUID + `","resourceVersion":"42"}}`
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedContentType = r.Header.Get("Content-Type")
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(serverResponse))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Create(context.Background(), pod)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, capturedMethod)
		assert.Equal(t, "application/json", capturedContentType)
		// Create path should not include the pod name (collection path)
		assert.False(t, strings.HasSuffix(capturedPath, "/"+testPodName), "create path %q should not end with resource name", capturedPath)
		// Server-assigned fields must be decoded back into the passed-in object
		assert.Equal(t, serverAssignedUID, string(pod.UID), "pod.UID should be set from server response")
		assert.Equal(t, "42", pod.ResourceVersion, "pod.ResourceVersion should be set from server response")
	})

	t.Run("409 conflict returns error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte("already exists"))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Create(context.Background(), pod)

		require.Error(t, err)
		assert.True(t, apierrors.IsConflict(err), "expected Conflict, got %T: %v", err, err)
	})
}

func TestProxyClientUpdate(t *testing.T) {
	t.Run("success with PUT method", func(t *testing.T) {
		var capturedMethod string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Update(context.Background(), pod)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPut, capturedMethod)
	})

	t.Run("404 not found returns error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Update(context.Background(), pod)

		require.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err), "expected NotFound, got %T: %v", err, err)
	})
}

func TestProxyClientPatch(t *testing.T) {
	t.Run("merge patch uses correct content-type", func(t *testing.T) {
		var capturedMethod, capturedContentType string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedContentType = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		patch := client.RawPatch(types.MergePatchType, []byte(`{"metadata":{"labels":{"x":"y"}}}`))
		err := pc.Patch(context.Background(), pod, patch)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPatch, capturedMethod)
		assert.Equal(t, string(types.MergePatchType), capturedContentType)
	})

	t.Run("patch with field manager adds query param", func(t *testing.T) {
		var capturedQuery string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedQuery = r.URL.RawQuery
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		patch := client.RawPatch(types.MergePatchType, []byte(`{}`))
		err := pc.Patch(context.Background(), pod, patch, &client.PatchOptions{FieldManager: "test-ctrl"})

		require.NoError(t, err)
		assert.Contains(t, capturedQuery, "fieldManager=test-ctrl")
	})
}

func TestProxyClientDelete(t *testing.T) {
	t.Run("200 OK is success", func(t *testing.T) {
		var capturedMethod string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			w.WriteHeader(http.StatusOK)
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Delete(context.Background(), pod)

		require.NoError(t, err)
		assert.Equal(t, http.MethodDelete, capturedMethod)
	})

	t.Run("404 is not an error for delete", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		// 404 on delete is intentionally treated as success
		assert.NoError(t, pc.Delete(context.Background(), pod))
	})

	t.Run("500 returns error", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal error"))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		require.Error(t, pc.Delete(context.Background(), pod))
	})
}

func TestProxyClientStatusUpdate(t *testing.T) {
	t.Run("status update uses PUT to /status path", func(t *testing.T) {
		var capturedMethod, capturedPath string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		err := pc.Status().Update(context.Background(), pod)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPut, capturedMethod)
		assert.True(t, strings.HasSuffix(capturedPath, "/status"), "path %q should end with /status", capturedPath)
	})

	t.Run("status create is not supported", func(t *testing.T) {
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {})
		pod := &corev1.Pod{}
		require.Error(t, pc.Status().Create(context.Background(), pod, pod))
	})
}

func TestProxyClientStatusPatch(t *testing.T) {
	t.Run("status patch uses PATCH to /status path", func(t *testing.T) {
		var capturedMethod, capturedPath, capturedContentType string
		pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {
			capturedMethod = r.Method
			capturedPath = r.URL.Path
			capturedContentType = r.Header.Get("Content-Type")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(testPodJSON))
		})

		pod := &corev1.Pod{}
		pod.Name = testPodName
		pod.Namespace = testPodNamespace
		patch := client.RawPatch(types.MergePatchType, []byte(`{"status":{}}`))
		err := pc.Status().Patch(context.Background(), pod, patch)

		require.NoError(t, err)
		assert.Equal(t, http.MethodPatch, capturedMethod)
		assert.True(t, strings.HasSuffix(capturedPath, "/status"), "path %q should end with /status", capturedPath)
		assert.Equal(t, string(types.MergePatchType), capturedContentType)
	})
}

func TestProxyClientStubs(t *testing.T) {
	pc := newTestProxyClient(t, func(w http.ResponseWriter, r *http.Request) {})

	t.Run("DeleteAllOf returns not-implemented error", func(t *testing.T) {
		err := pc.DeleteAllOf(context.Background(), &corev1.Pod{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
	})

	t.Run("SubResource.Get returns not-implemented", func(t *testing.T) {
		err := pc.SubResource("logs").Get(context.Background(), &corev1.Pod{}, &corev1.Pod{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
	})

	t.Run("SubResource.Create returns not-implemented", func(t *testing.T) {
		err := pc.SubResource("logs").Create(context.Background(), &corev1.Pod{}, &corev1.Pod{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
	})

	t.Run("SubResource.Update returns not-implemented", func(t *testing.T) {
		err := pc.SubResource("logs").Update(context.Background(), &corev1.Pod{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
	})

	t.Run("SubResource.Patch returns not-implemented", func(t *testing.T) {
		patch := client.RawPatch(types.MergePatchType, []byte(`{}`))
		err := pc.SubResource("logs").Patch(context.Background(), &corev1.Pod{}, patch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not implemented")
	})

	t.Run("Scheme returns non-nil", func(t *testing.T) {
		assert.NotNil(t, pc.Scheme())
	})

	t.Run("RESTMapper returns nil", func(t *testing.T) {
		assert.Nil(t, pc.RESTMapper())
	})

	t.Run("GroupVersionKindFor returns Pod GVK", func(t *testing.T) {
		gvk, err := pc.GroupVersionKindFor(&corev1.Pod{})
		require.NoError(t, err)
		assert.Equal(t, "Pod", gvk.Kind)
	})

	t.Run("IsObjectNamespaced Pod returns true", func(t *testing.T) {
		namespaced, err := pc.IsObjectNamespaced(&corev1.Pod{})
		require.NoError(t, err)
		assert.True(t, namespaced, "Pod should be namespaced")
	})

	t.Run("IsObjectNamespaced Namespace returns false", func(t *testing.T) {
		namespaced, err := pc.IsObjectNamespaced(&corev1.Namespace{})
		require.NoError(t, err)
		assert.False(t, namespaced, "Namespace resource should not be namespaced")
	})
}
