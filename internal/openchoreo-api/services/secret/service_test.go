// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/config"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	testNamespace  = "ns1"
	testSecretName = "my-secret"
	testLabelValue = "payments"
)

// externalSecretsGroup is the API group of the PushSecret CRD.
const externalSecretsGroup = "external-secrets.io"

// defaultRemoteKeys is the key layout a default install uses.
var defaultRemoteKeys = remoteKeyFormat{prefix: config.DefaultRemoteKeyPrefix, separator: config.DefaultRemoteKeySeparator}

func newTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := openchoreov1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add openchoreo scheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add corev1 scheme: %v", err)
	}
	return s
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func isValidationError(err error) bool {
	var v *services.ValidationError
	return errors.As(err, &v)
}

// --- validateSecretData ---

func TestValidateSecretData(t *testing.T) {
	tests := []struct {
		name       string
		secretType corev1.SecretType
		data       map[string]string
		wantErr    bool
	}{
		{
			name:       "opaque with one key succeeds",
			secretType: corev1.SecretTypeOpaque,
			data:       map[string]string{"token": "v"},
		},
		{
			name:       "opaque empty fails",
			secretType: corev1.SecretTypeOpaque,
			data:       map[string]string{},
			wantErr:    true,
		},
		{
			name:       "basic-auth with password succeeds",
			secretType: corev1.SecretTypeBasicAuth,
			data:       map[string]string{"password": "p", "username": "u"},
		},
		{
			name:       "basic-auth missing password fails",
			secretType: corev1.SecretTypeBasicAuth,
			data:       map[string]string{"username": "u"},
			wantErr:    true,
		},
		{
			name:       "basic-auth with password only succeeds",
			secretType: corev1.SecretTypeBasicAuth,
			data:       map[string]string{"password": "p"},
		},
		{
			name:       "ssh-auth with key succeeds",
			secretType: corev1.SecretTypeSSHAuth,
			data:       map[string]string{"ssh-privatekey": "k"},
		},
		{
			name:       "ssh-auth missing key fails",
			secretType: corev1.SecretTypeSSHAuth,
			data:       map[string]string{"ssh-key-id": "id"},
			wantErr:    true,
		},
		{
			name:       "dockerconfigjson with .dockerconfigjson succeeds",
			secretType: corev1.SecretTypeDockerConfigJson,
			data:       map[string]string{".dockerconfigjson": "{}"},
		},
		{
			name:       "dockerconfigjson missing .dockerconfigjson fails",
			secretType: corev1.SecretTypeDockerConfigJson,
			data:       map[string]string{"other": "v"},
			wantErr:    true,
		},
		{
			name:       "tls with both keys succeeds",
			secretType: corev1.SecretTypeTLS,
			data:       map[string]string{"tls.crt": "c", "tls.key": "k"},
		},
		{
			name:       "tls missing tls.key fails",
			secretType: corev1.SecretTypeTLS,
			data:       map[string]string{"tls.crt": "c"},
			wantErr:    true,
		},
		{
			name:       "unsupported type fails",
			secretType: corev1.SecretType("kubernetes.io/service-account-token"),
			data:       map[string]string{"token": "v"},
			wantErr:    true,
		},
		{
			name:       "empty value fails",
			secretType: corev1.SecretTypeOpaque,
			data:       map[string]string{"key": ""},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecretData(tt.secretType, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !isValidationError(err) {
					t.Fatalf("expected ValidationError, got %T: %v", err, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// --- validatePlaneKind ---

func TestValidatePlaneKind(t *testing.T) {
	for _, kind := range []string{
		planeKindWorkflowPlane,
		planeKindClusterWorkflowPlane,
		planeKindDataPlane,
		planeKindClusterDataPlane,
	} {
		if err := validatePlaneKind(kind); err != nil {
			t.Errorf("expected %s to be valid, got %v", kind, err)
		}
	}
	if err := validatePlaneKind("Bogus"); err == nil || !isValidationError(err) {
		t.Errorf("expected validation error for Bogus, got %v", err)
	}
	if err := validatePlaneKind(""); err == nil || !isValidationError(err) {
		t.Errorf("expected validation error for empty kind, got %v", err)
	}
}

// --- validateSecretName ---

func TestValidateSecretName(t *testing.T) {
	if err := validateSecretName(""); err == nil || !isValidationError(err) {
		t.Errorf("expected validation error for empty name, got %v", err)
	}
	if err := validateSecretName("ok"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- helpers ---

func TestKvNamespace(t *testing.T) {
	if got := kvNamespace("foo"); got != "openchoreo-kv-foo" {
		t.Errorf("kvNamespace(foo) = %q, want openchoreo-kv-foo", got)
	}
}

func TestRemoteKeyFor(t *testing.T) {
	tests := []struct {
		name       string
		secretType corev1.SecretType
		want       string
	}{
		{"opaque", corev1.SecretTypeOpaque, "secret/ns1/generic/my-secret"},
		{"basic-auth", corev1.SecretTypeBasicAuth, "secret/ns1/basic-auth/my-secret"},
		{"ssh-auth", corev1.SecretTypeSSHAuth, "secret/ns1/ssh-auth/my-secret"},
		{"dockerconfigjson", corev1.SecretTypeDockerConfigJson, "secret/ns1/registry/my-secret"},
		{"tls", corev1.SecretTypeTLS, "secret/ns1/tls/my-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remoteKeyFor(defaultRemoteKeys, testNamespace, tt.secretType, testSecretName); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortedKeys(t *testing.T) {
	got := sortedKeys(map[string]string{"b": "1", "a": "2", "c": "3"})
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- builders ---

func TestBuildK8sSecret(t *testing.T) {
	s := buildK8sSecret(testSecretName, kvNamespace(testNamespace), corev1.SecretTypeBasicAuth,
		map[string]string{"password": "p"})
	if s.Name != testSecretName {
		t.Errorf("Name = %q", s.Name)
	}
	if s.Namespace != kvNamespace(testNamespace) {
		t.Errorf("Namespace = %q", s.Namespace)
	}
	if s.Type != corev1.SecretTypeBasicAuth {
		t.Errorf("Type = %v", s.Type)
	}
	if string(s.Data["password"]) != "p" {
		t.Errorf("Data[password] = %q, want %q", s.Data["password"], "p")
	}
	if s.StringData != nil {
		t.Errorf("StringData should be unset (use Data so SSA can prune keys)")
	}
}

func TestBuildPushSecret(t *testing.T) {
	ps := buildPushSecret(defaultRemoteKeys, testSecretName, testNamespace, kvNamespace(testNamespace), "vault-store", corev1.SecretTypeOpaque)

	if ps.GetAPIVersion() != pushSecretAPIVersion {
		t.Errorf("apiVersion = %q", ps.GetAPIVersion())
	}
	if ps.GetKind() != pushSecretKind {
		t.Errorf("kind = %q", ps.GetKind())
	}
	if ps.GetName() != testSecretName {
		t.Errorf("name = %q", ps.GetName())
	}
	if ps.GetNamespace() != kvNamespace(testNamespace) {
		t.Errorf("namespace = %q", ps.GetNamespace())
	}
	if ann := ps.GetAnnotations()[syncTriggerAnnotation]; ann == "" {
		t.Errorf("expected %s annotation to be set", syncTriggerAnnotation)
	}

	spec, ok := ps.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("spec is not a map")
	}
	if spec["updatePolicy"] != "Replace" {
		t.Errorf("updatePolicy = %v", spec["updatePolicy"])
	}
	if spec["deletionPolicy"] != "Delete" {
		t.Errorf("deletionPolicy = %v", spec["deletionPolicy"])
	}
	storeRefs := spec["secretStoreRefs"].([]map[string]any)
	if storeRefs[0]["name"] != "vault-store" {
		t.Errorf("store name = %v", storeRefs[0]["name"])
	}
	// One entry with only the remote key pushes the whole Secret as one value.
	data := spec["data"].([]map[string]any)
	if len(data) != 1 {
		t.Fatalf("data length = %d, want 1", len(data))
	}
	match := data[0]["match"].(map[string]any)
	if _, ok := match["secretKey"]; ok {
		t.Errorf("secretKey = %v, want none", match["secretKey"])
	}
	remoteRef := match["remoteRef"].(map[string]any)
	if remoteRef["remoteKey"] != "secret/ns1/generic/my-secret" {
		t.Errorf("remoteKey = %v", remoteRef["remoteKey"])
	}
	if _, ok := remoteRef["property"]; ok {
		t.Errorf("property = %v, want none", remoteRef["property"])
	}
}

func TestBuildSecretReference(t *testing.T) {
	ref := buildSecretReference(defaultRemoteKeys, testNamespace, testSecretName, corev1.SecretTypeBasicAuth,
		openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
		[]string{"password", "username"},
		map[string]string{"team": testLabelValue})

	if ref.Name != testSecretName || ref.Namespace != testNamespace {
		t.Errorf("metadata = %s/%s", ref.Namespace, ref.Name)
	}
	if ref.Spec.TargetPlane == nil || ref.Spec.TargetPlane.Kind != planeKindWorkflowPlane || ref.Spec.TargetPlane.Name != "wp1" {
		t.Errorf("spec.targetPlane = %+v", ref.Spec.TargetPlane)
	}
	if ref.Spec.Template.Type != corev1.SecretTypeBasicAuth {
		t.Errorf("template.type = %v", ref.Spec.Template.Type)
	}
	if len(ref.Spec.Data) != 2 {
		t.Fatalf("data length = %d", len(ref.Spec.Data))
	}
	wantKey := "secret/ns1/basic-auth/my-secret"
	for _, d := range ref.Spec.Data {
		if d.RemoteRef.Key != wantKey {
			t.Errorf("data.remoteRef.key = %q, want %q", d.RemoteRef.Key, wantKey)
		}
		if d.RemoteRef.Property != d.SecretKey {
			t.Errorf("data.remoteRef.property = %q, want %q", d.RemoteRef.Property, d.SecretKey)
		}
	}
	if ref.Labels[managedByLabel] != managedByOpenchoreoAPI {
		t.Errorf("managed-by label = %q", ref.Labels[managedByLabel])
	}
	if ref.Labels["team"] != testLabelValue {
		t.Errorf("user label team = %q, want payments", ref.Labels["team"])
	}
}

// --- resolvePlane ---

func TestResolvePlane_WorkflowPlane(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	targetClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: k8sClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	info, err := svc.resolvePlane(context.Background(), testNamespace, planeKindWorkflowPlane, "wp1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.secretStoreName != "store" {
		t.Errorf("secretStoreName = %q", info.secretStoreName)
	}
	if info.k8sClient != targetClient {
		t.Errorf("expected target client to be returned")
	}
}

func TestResolvePlane_ClusterDataPlane(t *testing.T) {
	scheme := newTestScheme(t)
	cdp := &openchoreov1alpha1.ClusterDataPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "cdp1"},
		Spec: openchoreov1alpha1.ClusterDataPlaneSpec{
			PlaneID:        "main",
			ClusterAgent:   openchoreov1alpha1.ClusterAgentConfig{},
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cdp).Build()

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	targetClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	mockProvider.EXPECT().ClusterDataPlaneClient(cdp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: k8sClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	info, err := svc.resolvePlane(context.Background(), testNamespace, planeKindClusterDataPlane, "cdp1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.secretStoreName != "store" {
		t.Errorf("secretStoreName = %q", info.secretStoreName)
	}
}

func TestResolvePlane_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	for _, kind := range []string{
		planeKindWorkflowPlane, planeKindClusterWorkflowPlane,
		planeKindDataPlane, planeKindClusterDataPlane,
	} {
		_, err := svc.resolvePlane(context.Background(), testNamespace, kind, "missing")
		if !errors.Is(err, ErrPlaneNotFound) {
			t.Errorf("kind %s: expected ErrPlaneNotFound, got %v", kind, err)
		}
	}
}

func TestResolvePlane_NoSecretStore(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.resolvePlane(context.Background(), testNamespace, planeKindWorkflowPlane, "wp1")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

func TestResolvePlane_InvalidKind(t *testing.T) {
	svc := &secretService{logger: newTestLogger(), remoteKeys: defaultRemoteKeys}
	_, err := svc.resolvePlane(context.Background(), testNamespace, "Bogus", "x")
	if !isValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

// --- ensureNamespaceExists ---

func TestEnsureNamespaceExists_AlreadyExists(t *testing.T) {
	scheme := newTestScheme(t)
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "openchoreo-kv-ns1"}}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ns).Build()
	svc := &secretService{logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	if err := svc.ensureNamespaceExists(context.Background(), k8sClient, "openchoreo-kv-ns1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNamespaceExists_CreatesNew(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	if err := svc.ensureNamespaceExists(context.Background(), k8sClient, "openchoreo-kv-ns1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := &corev1.Namespace{}
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "openchoreo-kv-ns1"}, got); err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
}

// --- CreateSecret error paths (no SSA required) ---

func TestCreateSecret_AlreadyExists(t *testing.T) {
	scheme := newTestScheme(t)
	existing := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.CreateSecret(context.Background(), testNamespace, &CreateSecretParams{
		SecretName:  testSecretName,
		SecretType:  corev1.SecretTypeOpaque,
		TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
		Data:        map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrSecretAlreadyExists) {
		t.Errorf("expected ErrSecretAlreadyExists, got %v", err)
	}
}

func TestCreateSecret_ValidationErrors(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	cases := []struct {
		name string
		req  *CreateSecretParams
	}{
		{
			name: "empty name",
			req: &CreateSecretParams{
				SecretType:  corev1.SecretTypeOpaque,
				TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
				Data:        map[string]string{"k": "v"},
			},
		},
		{
			name: "missing required key",
			req: &CreateSecretParams{
				SecretName:  testSecretName,
				SecretType:  corev1.SecretTypeBasicAuth,
				TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
				Data:        map[string]string{"username": "u", "other": "v"}, // missing password
			},
		},
		{
			name: "bogus plane kind",
			req: &CreateSecretParams{
				SecretName:  testSecretName,
				SecretType:  corev1.SecretTypeOpaque,
				TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: "Bogus", Name: "wp1"},
				Data:        map[string]string{"k": "v"},
			},
		},
		{
			name: "missing plane name",
			req: &CreateSecretParams{
				SecretName:  testSecretName,
				SecretType:  corev1.SecretTypeOpaque,
				TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: ""},
				Data:        map[string]string{"k": "v"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateSecret(context.Background(), testNamespace, tc.req)
			if !isValidationError(err) {
				t.Errorf("expected validation error, got %v", err)
			}
		})
	}
}

func TestCreateSecret_PlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.CreateSecret(context.Background(), testNamespace, &CreateSecretParams{
		SecretName:  testSecretName,
		SecretType:  corev1.SecretTypeOpaque,
		TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "missing"},
		Data:        map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrPlaneNotFound) {
		t.Errorf("expected ErrPlaneNotFound, got %v", err)
	}
}

// --- DeleteSecret error paths ---

func TestDeleteSecret_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	err := svc.DeleteSecret(context.Background(), testNamespace, "missing")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestDeleteSecret_NoTargetPlane(t *testing.T) {
	scheme := newTestScheme(t)
	legacy := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	err := svc.DeleteSecret(context.Background(), testNamespace, "legacy")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestDeleteSecret_PlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
			Labels:    map[string]string{managedByLabel: managedByOpenchoreoAPI},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "missing"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	err := svc.DeleteSecret(context.Background(), testNamespace, testSecretName)
	if !errors.Is(err, ErrPlaneNotFound) {
		t.Errorf("expected ErrPlaneNotFound, got %v", err)
	}
}

// --- happy paths via interceptor (SSA Patch + unstructured PushSecret) ---

// targetPlaneClient builds a fake client that:
//   - swallows SSA Patch calls (records count)
//   - returns NotFound for unstructured (PushSecret) Get and Delete
//
// Real Get and Delete on typed objects still go through the underlying tracker.
type patchRec struct {
	count int
}

func newTargetPlaneClient(t *testing.T, scheme *runtime.Scheme, objs []client.Object, rec *patchRec) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				// No PushSecret exists yet in the target plane.
				if gvk := obj.GetObjectKind().GroupVersionKind(); gvk.Group == externalSecretsGroup {
					return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: "pushsecrets"}, key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			},
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				if rec != nil {
					rec.count++
				}
				return nil
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				// PushSecret is an unstructured external CRD; surface NotFound for it
				// so the service treats it as already-gone.
				if obj.GetObjectKind().GroupVersionKind().Group == externalSecretsGroup {
					gvk := obj.GetObjectKind().GroupVersionKind()
					return apierrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: "pushsecrets"}, obj.GetName())
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
}

func TestCreateSecret_Success(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp).Build()

	rec := &patchRec{}
	targetClient := newTargetPlaneClient(t, scheme, nil, rec)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	info, err := svc.CreateSecret(context.Background(), testNamespace, &CreateSecretParams{
		SecretName:  testSecretName,
		SecretType:  corev1.SecretTypeBasicAuth,
		TargetPlane: openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
		Data:        map[string]string{"password": "p", "username": "u"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Name != testSecretName {
		t.Errorf("info.Name = %q", info.Name)
	}
	if len(info.Data) != 2 {
		t.Errorf("info.Data has %d keys, want 2", len(info.Data))
	}
	if rec.count != 2 {
		t.Errorf("expected 2 SSA patches (Secret + PushSecret), got %d", rec.count)
	}

	// SecretReference must exist with TargetPlane populated.
	got := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("SecretReference not created: %v", err)
	}
	if got.Spec.TargetPlane == nil || got.Spec.TargetPlane.Name != "wp1" {
		t.Errorf("spec.targetPlane = %+v", got.Spec.TargetPlane)
	}

	// Target plane namespace must have been created.
	ns := &corev1.Namespace{}
	if err := targetClient.Get(context.Background(),
		client.ObjectKey{Name: kvNamespace(testNamespace)}, ns); err != nil {
		t.Errorf("kv namespace not created: %v", err)
	}
}

func TestDeleteSecret_Success(t *testing.T) {
	scheme := newTestScheme(t)

	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNamespace,
			Labels:    map[string]string{managedByLabel: managedByOpenchoreoAPI},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
			Data: []openchoreov1alpha1.SecretDataSource{{
				SecretKey: "k",
				RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/ns1/generic/my-secret", Property: "k"},
			}},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	targetSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: kvNamespace(testNamespace)},
	}
	targetClient := newTargetPlaneClient(t, scheme, []client.Object{targetSecret}, nil)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	if err := svc.DeleteSecret(context.Background(), testNamespace, testSecretName); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, got); err == nil {
		t.Error("expected SecretReference to be deleted")
	}
	gotSecret := &corev1.Secret{}
	if err := targetClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: kvNamespace(testNamespace)}, gotSecret); err == nil {
		t.Error("expected target plane Secret to be deleted")
	}
}

// --- GetSecret ---

func managedRef(name string, secretType corev1.SecretType, keys []string) *openchoreov1alpha1.SecretReference {
	dataSources := make([]openchoreov1alpha1.SecretDataSource, 0, len(keys))
	for _, k := range keys {
		dataSources = append(dataSources, openchoreov1alpha1.SecretDataSource{
			SecretKey: k,
			RemoteRef: openchoreov1alpha1.RemoteReference{
				Key: remoteKeyFor(defaultRemoteKeys, testNamespace, secretType, name), Property: k,
			},
		})
	}
	return &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels:    map[string]string{managedByLabel: managedByOpenchoreoAPI},
		},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: secretType},
			Data:        dataSources,
		},
	}
}

func newWorkflowPlane() *openchoreov1alpha1.WorkflowPlane {
	return &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
}

func TestGetSecret_Success(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeBasicAuth, []string{"password", "username"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	planeSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: kvNamespace(testNamespace)},
		Type:       corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			"username": []byte("alice"),
			"password": []byte("s3cret"),
		},
	}
	targetClient := newTargetPlaneClient(t, scheme, []client.Object{planeSecret}, nil)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	got, err := svc.GetSecret(context.Background(), testNamespace, testSecretName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != testSecretName || got.Namespace != testNamespace {
		t.Errorf("metadata mismatch: %s/%s", got.Namespace, got.Name)
	}
	if got.Type != corev1.SecretTypeBasicAuth {
		t.Errorf("type = %q", got.Type)
	}
	if string(got.Data["username"]) != "alice" || string(got.Data["password"]) != "s3cret" {
		t.Errorf("data = %v", got.Data)
	}
}

func TestGetSecret_NotFound_Missing(t *testing.T) {
	scheme := newTestScheme(t)
	cpClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: cpClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.GetSecret(context.Background(), testNamespace, testSecretName)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestGetSecret_NotFound_UnmanagedRef(t *testing.T) {
	scheme := newTestScheme(t)
	// SecretReference exists but has no managed-by label.
	unmanaged := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(unmanaged).Build()
	svc := &secretService{k8sClient: cpClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.GetSecret(context.Background(), testNamespace, testSecretName)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound for unmanaged SecretReference, got %v", err)
	}
}

func TestGetSecret_NotFound_TargetPlaneSecretMissing(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"k"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	// Target plane has no Secret yet.
	targetClient := newTargetPlaneClient(t, scheme, nil, nil)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.GetSecret(context.Background(), testNamespace, testSecretName)
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound when target Secret missing, got %v", err)
	}
}

// --- ListSecrets ---

func TestListSecrets_FiltersManagedByLabel(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	managed := managedRef("managed", corev1.SecretTypeOpaque, []string{"k"})
	unmanaged := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: "unmanaged", Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, managed, unmanaged).Build()

	managedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "managed", Namespace: kvNamespace(testNamespace)},
		Data:       map[string][]byte{"k": []byte("v")},
	}
	targetClient := newTargetPlaneClient(t, scheme, []client.Object{managedSecret}, nil)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	result, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "managed" {
		t.Errorf("expected only managed secret, got %+v", result.Items)
	}
	if string(result.Items[0].Data["k"]) != "v" {
		t.Errorf("data not propagated: %v", result.Items[0].Data)
	}
}

func TestListSecrets_SkipsMissingTargetSecret(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"k"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	// Target plane has no Secret.
	targetClient := newTargetPlaneClient(t, scheme, nil, nil)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	result, err := svc.ListSecrets(context.Background(), testNamespace, services.ListOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected items to be skipped, got %d", len(result.Items))
	}
}

// --- UpdateSecret ---

func TestUpdateSecret_Success_SameKeys(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	rec := &patchRec{}
	targetClient := newTargetPlaneClient(t, scheme, nil, rec)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	got, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"a": "1", "b": "2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.count != 2 {
		t.Errorf("expected 2 SSA patches, got %d", rec.count)
	}
	if string(got.Data["a"]) != "1" {
		t.Errorf("data not propagated: %v", got.Data)
	}

	// The SecretReference data section is unchanged on same-key updates.
	updated := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("get SecretReference: %v", err)
	}
	got2 := dataSourceKeys(updated.Spec.Data)
	want := []string{"a", "b"}
	if !sameStringSlice(got2, want) {
		t.Errorf("spec.data keys = %v, want %v", got2, want)
	}
}

func TestUpdateSecret_Success_KeysChanged(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	targetClient := newTargetPlaneClient(t, scheme, nil, &patchRec{})

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"a": "1", "c": "3"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("get SecretReference: %v", err)
	}
	got := dataSourceKeys(updated.Spec.Data)
	want := []string{"a", "c"}
	if !sameStringSlice(got, want) {
		t.Errorf("spec.data keys = %v, want %v", got, want)
	}
}

// servedPushSecret returns ps the way the API server serves it, with
// JSON-typed values, so it can be deep-copied and read with the unstructured
// helpers.
func servedPushSecret(t *testing.T, ps *unstructured.Unstructured) *unstructured.Unstructured {
	t.Helper()
	raw, err := json.Marshal(ps.Object)
	if err != nil {
		t.Fatalf("marshal push secret: %v", err)
	}
	out := &unstructured.Unstructured{}
	if err := out.UnmarshalJSON(raw); err != nil {
		t.Fatalf("unmarshal push secret: %v", err)
	}
	return out
}

// perKeyPushSecret returns a PushSecret in the shape created before secrets
// were pushed as one entry: one data entry per key, each with a property.
func perKeyPushSecret(t *testing.T, keys ...string) *unstructured.Unstructured {
	t.Helper()
	ps := servedPushSecret(t, buildPushSecret(defaultRemoteKeys, testSecretName, testNamespace,
		kvNamespace(testNamespace), "store", corev1.SecretTypeOpaque))
	entries := make([]any, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, map[string]any{"match": map[string]any{
			"secretKey": k,
			"remoteRef": map[string]any{"remoteKey": "secret/ns1/generic/my-secret", "property": k},
		}})
	}
	if err := unstructured.SetNestedSlice(ps.Object, entries, "spec", "data"); err != nil {
		t.Fatalf("set data: %v", err)
	}
	ps.SetFinalizers([]string{"pushsecret.externalsecrets.io/finalizer"})
	return ps
}

// newRecordingPlaneClient builds a fake target plane client that serves
// existing as the current PushSecret (nil means none) and records each write
// in order.
func newRecordingPlaneClient(t *testing.T, scheme *runtime.Scheme, existing *unstructured.Unstructured, calls *[]string) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				u, ok := obj.(*unstructured.Unstructured)
				if !ok || u.GroupVersionKind().Group != externalSecretsGroup {
					return c.Get(ctx, key, obj, opts...)
				}
				if existing == nil {
					return apierrors.NewNotFound(schema.GroupResource{Group: externalSecretsGroup, Resource: "pushsecrets"}, key.Name)
				}
				u.Object = existing.DeepCopy().Object
				return nil
			},
			Patch: func(_ context.Context, _ client.WithWatch, obj client.Object, patch client.Patch, _ ...client.PatchOption) error {
				kind := obj.GetObjectKind().GroupVersionKind().Kind
				if patch.Type() == client.Apply.Type() {
					*calls = append(*calls, "apply "+kind)
					return nil
				}
				data, err := patch.Data(obj)
				if err != nil {
					return err
				}
				*calls = append(*calls, "patch "+kind+" "+string(data))
				return nil
			},
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				*calls = append(*calls, "delete "+obj.GetObjectKind().GroupVersionKind().Kind)
				return nil
			},
		}).
		Build()
}

// TestUpdateSecret_ReplacesPerKeyPushSecret checks that a PushSecret with one
// entry per key is dropped with deletion turned off and its finalizer removed
// before the whole-secret PushSecret is applied, so external-secrets does not
// delete the existing store entry.
func TestUpdateSecret_ReplacesPerKeyPushSecret(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	var calls []string
	targetClient := newRecordingPlaneClient(t, scheme, perKeyPushSecret(t, "a", "b"), &calls)
	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}
	if _, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"a": "1", "b": "2"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"apply Secret",
		`patch PushSecret {"spec":{"deletionPolicy":"None"},"metadata":{"finalizers":null}}`,
		"delete PushSecret",
		"apply PushSecret",
	}
	if !slices.Equal(calls, want) {
		t.Errorf("target plane writes = %q, want %q", calls, want)
	}
}

// TestUpdateSecret_KeepsWholeSecretPushSecret checks that a PushSecret that
// already pushes the whole secret is applied in place.
func TestUpdateSecret_KeepsWholeSecretPushSecret(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	existing := servedPushSecret(t, buildPushSecret(defaultRemoteKeys, testSecretName, testNamespace,
		kvNamespace(testNamespace), "store", corev1.SecretTypeOpaque))
	var calls []string
	targetClient := newRecordingPlaneClient(t, scheme, existing, &calls)
	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}
	if _, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"a": "1"},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"apply Secret", "apply PushSecret"}
	if !slices.Equal(calls, want) {
		t.Errorf("target plane writes = %q, want %q", calls, want)
	}
}

func TestHasPerKeyEntries(t *testing.T) {
	whole := servedPushSecret(t, buildPushSecret(defaultRemoteKeys, testSecretName, testNamespace,
		kvNamespace(testNamespace), "store", corev1.SecretTypeOpaque))
	propertyOnly := perKeyPushSecret(t, "a")
	unstructured.RemoveNestedField(propertyOnly.Object, "spec", "data")
	if err := unstructured.SetNestedSlice(propertyOnly.Object, []any{map[string]any{"match": map[string]any{
		"remoteRef": map[string]any{"remoteKey": "secret/ns1/generic/my-secret", "property": "a"},
	}}}, "spec", "data"); err != nil {
		t.Fatalf("set data: %v", err)
	}
	noData := whole.DeepCopy()
	unstructured.RemoveNestedField(noData.Object, "spec", "data")

	tests := []struct {
		name string
		ps   *unstructured.Unstructured
		want bool
	}{
		{"whole secret", whole, false},
		{"one entry per key", perKeyPushSecret(t, "a", "b"), true},
		{"property without secretKey", propertyOnly, true},
		{"no data", noData, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasPerKeyEntries(tt.ps); got != tt.want {
				t.Errorf("hasPerKeyEntries = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateSecret_LabelsApplied(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	targetClient := newTargetPlaneClient(t, scheme, nil, &patchRec{})
	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data:   map[string]string{"a": "1", "b": "2"},
		Labels: map[string]string{"team": testLabelValue},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("get SecretReference: %v", err)
	}
	if updated.Labels["team"] != testLabelValue {
		t.Errorf("user label team = %q, want payments", updated.Labels["team"])
	}
	if updated.Labels[managedByLabel] != managedByOpenchoreoAPI {
		t.Errorf("managed-by label = %q, want preserved", updated.Labels[managedByLabel])
	}
}

func TestUpdateSecret_LabelsReplacedAndManagedByPreserved(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	ref.Labels["stale"] = "yes"
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	targetClient := newTargetPlaneClient(t, scheme, nil, &patchRec{})
	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data:   map[string]string{"a": "1", "b": "2"},
		Labels: map[string]string{"team": testLabelValue},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, updated); err != nil {
		t.Fatalf("get SecretReference: %v", err)
	}
	if _, ok := updated.Labels["stale"]; ok {
		t.Errorf("stale label should have been replaced, labels = %v", updated.Labels)
	}
	if updated.Labels[managedByLabel] != managedByOpenchoreoAPI {
		t.Errorf("managed-by label = %q, want preserved", updated.Labels[managedByLabel])
	}
}

func TestUpdateSecret_InvalidLabel(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeOpaque, []string{"a", "b"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()
	svc := &secretService{k8sClient: cpClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data:   map[string]string{"a": "1", "b": "2"},
		Labels: map[string]string{"bad key": "v"},
	})
	var vErr *services.ValidationError
	if !errors.As(err, &vErr) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestValidateLabels(t *testing.T) {
	tests := []struct {
		name    string
		labels  map[string]string
		wantErr bool
	}{
		{"nil", nil, false},
		{"empty", map[string]string{}, false},
		{"valid", map[string]string{"team": testLabelValue, "app.kubernetes.io/name": "x"}, false},
		{"invalid key", map[string]string{"bad key": "v"}, true},
		{"invalid value", map[string]string{"k": "bad value!"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabels(tt.labels)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMergeManagedLabels(t *testing.T) {
	// The managed-by label is always set, even if the caller tries to override it.
	got := mergeManagedLabels(map[string]string{
		"team":         testLabelValue,
		managedByLabel: "someone-else",
	})
	if got[managedByLabel] != managedByOpenchoreoAPI {
		t.Errorf("managed-by label = %q, want %q", got[managedByLabel], managedByOpenchoreoAPI)
	}
	if got["team"] != testLabelValue {
		t.Errorf("team label = %q, want payments", got["team"])
	}

	// A nil input still yields the managed-by label.
	got = mergeManagedLabels(nil)
	if got[managedByLabel] != managedByOpenchoreoAPI {
		t.Errorf("managed-by label = %q, want %q", got[managedByLabel], managedByOpenchoreoAPI)
	}
}

func TestUpdateSecret_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	cpClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: cpClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestUpdateSecret_ValidationFailure(t *testing.T) {
	scheme := newTestScheme(t)
	wp := newWorkflowPlane()
	ref := managedRef(testSecretName, corev1.SecretTypeBasicAuth, []string{"password", "username"})
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()
	svc := &secretService{k8sClient: cpClient, logger: newTestLogger(), remoteKeys: defaultRemoteKeys}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		// Missing required password key for basic-auth.
		Data: map[string]string{"username": "u", "other": "v"},
	})
	if !isValidationError(err) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestRemoteKeyForPrefixes(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"default prefix", config.DefaultRemoteKeyPrefix, "secret/ns1/generic/my-secret"},
		{"empty prefix drops segment", "", "ns1/generic/my-secret"},
		{"custom prefix", "openchoreo", "openchoreo/ns1/generic/my-secret"},
		{"nested prefix", "oc/secrets", "oc/secrets/ns1/generic/my-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := remoteKeyFormat{prefix: tt.prefix, separator: config.DefaultRemoteKeySeparator}
			if got := remoteKeyFor(format, testNamespace, corev1.SecretTypeOpaque, testSecretName); got != tt.want {
				t.Errorf("remoteKeyFor(%q) = %q, want %q", tt.prefix, got, tt.want)
			}
		})
	}
}

// TestRemoteKeyForDashSeparator checks the flat layout used for Azure Key
// Vault: every segment, including those of a nested prefix, is joined with
// "-", so no "/" is left in the key.
func TestRemoteKeyForDashSeparator(t *testing.T) {
	tests := []struct {
		name       string
		prefix     string
		secretType corev1.SecretType
		want       string
	}{
		{"default prefix", config.DefaultRemoteKeyPrefix, corev1.SecretTypeOpaque, "secret-ns1-generic-my-secret"},
		{"empty prefix drops segment", "", corev1.SecretTypeOpaque, "ns1-generic-my-secret"},
		{"nested prefix", "oc/secrets", corev1.SecretTypeOpaque, "oc-secrets-ns1-generic-my-secret"},
		{"basic-auth", config.DefaultRemoteKeyPrefix, corev1.SecretTypeBasicAuth, "secret-ns1-basic-auth-my-secret"},
		{"dockerconfigjson", config.DefaultRemoteKeyPrefix, corev1.SecretTypeDockerConfigJson, "secret-ns1-registry-my-secret"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			format := remoteKeyFormat{prefix: tt.prefix, separator: "-"}
			if got := remoteKeyFor(format, testNamespace, tt.secretType, testSecretName); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRemoteKeyForOtherSeparators checks that every other accepted separator
// joins all segments, including those of a nested prefix, and nothing else.
func TestRemoteKeyForOtherSeparators(t *testing.T) {
	for _, sep := range []string{"_", ".", "+", "=", "@"} {
		t.Run(sep, func(t *testing.T) {
			format := remoteKeyFormat{prefix: "oc/secrets", separator: sep}
			want := "oc" + sep + "secrets" + sep + "ns1" + sep + "generic" + sep + "my-secret"
			if got := remoteKeyFor(format, testNamespace, corev1.SecretTypeOpaque, testSecretName); got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestBuildersHonorEmptyPrefix pins the fix for the duplicate-entry bug: with an
// empty prefix the PushSecret and the SecretReference must agree on a key that
// carries no leading segment, so the key cannot collide with the KV mount name.
func TestBuildersHonorEmptyPrefix(t *testing.T) {
	assertBuildersAgreeOnKey(t, remoteKeyFormat{separator: config.DefaultRemoteKeySeparator}, "ns1/generic/my-secret")
}

// TestBuildersHonorDashSeparator checks that the PushSecret and the
// SecretReference agree on the flat key, so consumers read from the entry
// that external-secrets writes.
func TestBuildersHonorDashSeparator(t *testing.T) {
	assertBuildersAgreeOnKey(t, remoteKeyFormat{prefix: config.DefaultRemoteKeyPrefix, separator: "-"}, "secret-ns1-generic-my-secret")
}

func assertBuildersAgreeOnKey(t *testing.T, format remoteKeyFormat, wantKey string) {
	t.Helper()

	ps := buildPushSecret(format, testSecretName, testNamespace, kvNamespace(testNamespace), "vault-store", corev1.SecretTypeOpaque)
	spec, ok := ps.Object["spec"].(map[string]any)
	if !ok {
		t.Fatal("spec is not a map")
	}
	matches, ok := spec["data"].([]map[string]any)
	if !ok || len(matches) != 1 {
		t.Fatalf("data = %#v", spec["data"])
	}
	match := matches[0]["match"].(map[string]any)
	remoteRef := match["remoteRef"].(map[string]any)
	if remoteRef["remoteKey"] != wantKey {
		t.Errorf("PushSecret remoteKey = %v, want %q", remoteRef["remoteKey"], wantKey)
	}

	ref := buildSecretReference(format, testNamespace, testSecretName, corev1.SecretTypeOpaque,
		openchoreov1alpha1.TargetPlaneRef{Kind: planeKindDataPlane, Name: "dp1"}, []string{"a"}, nil)
	if len(ref.Spec.Data) != 1 {
		t.Fatalf("data sources = %d", len(ref.Spec.Data))
	}
	if ref.Spec.Data[0].RemoteRef.Key != wantKey {
		t.Errorf("SecretReference key = %q, want %q", ref.Spec.Data[0].RemoteRef.Key, wantKey)
	}
}

// TestNewServiceNormalizesPrefix checks that stray slashes in the configured
// prefix cannot produce empty or doubled segments in the key.
func TestNewServiceNormalizesPrefix(t *testing.T) {
	tests := []struct {
		configured string
		want       string
	}{
		{"secret", "secret/ns1/generic/my-secret"},
		{"/secret/", "secret/ns1/generic/my-secret"},
		{"team//prod", "team/prod/ns1/generic/my-secret"},
		{"//team///prod//", "team/prod/ns1/generic/my-secret"},
		{"", "ns1/generic/my-secret"},
		{"/", "ns1/generic/my-secret"},
	}
	for _, tt := range tests {
		cfg := config.SecretManagementConfig{
			Enabled:            true,
			RemoteKeyPrefix:    tt.configured,
			RemoteKeySeparator: config.DefaultRemoteKeySeparator,
		}
		svc, ok := NewService(nil, nil, cfg, newTestLogger()).(*secretService)
		if !ok {
			t.Fatal("NewService did not return *secretService")
		}
		got := remoteKeyFor(svc.remoteKeys, testNamespace, corev1.SecretTypeOpaque, testSecretName)
		if got != tt.want {
			t.Errorf("prefix %q: key = %q, want %q", tt.configured, got, tt.want)
		}
	}
}

// TestNewServiceAppliesSeparator checks that the configured separator reaches
// the service and also joins the segments of a nested prefix.
func TestNewServiceAppliesSeparator(t *testing.T) {
	cfg := config.SecretManagementConfig{
		Enabled:            true,
		RemoteKeyPrefix:    "//team///prod//",
		RemoteKeySeparator: "-",
	}
	svc, ok := NewService(nil, nil, cfg, newTestLogger()).(*secretService)
	if !ok {
		t.Fatal("NewService did not return *secretService")
	}
	const want = "team-prod-ns1-generic-my-secret"
	if got := remoteKeyFor(svc.remoteKeys, testNamespace, corev1.SecretTypeOpaque, testSecretName); got != want {
		t.Errorf("key = %q, want %q", got, want)
	}
}
