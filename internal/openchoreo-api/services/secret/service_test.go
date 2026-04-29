// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	k8sMocks "github.com/openchoreo/openchoreo/internal/clients/kubernetes/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
)

const (
	testNamespace  = "ns1"
	testSecretName = "my-secret"
)

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
			name:       "basic-auth missing username fails",
			secretType: corev1.SecretTypeBasicAuth,
			data:       map[string]string{"password": "p"},
			wantErr:    true,
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
			if got := remoteKeyFor(testNamespace, tt.secretType, testSecretName); got != tt.want {
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

func TestSameKeySet(t *testing.T) {
	existing := []openchoreov1alpha1.SecretDataSource{
		{SecretKey: "a"}, {SecretKey: "b"},
	}
	if !sameKeySet(existing, []string{"a", "b"}) {
		t.Error("expected same keys")
	}
	if !sameKeySet(existing, []string{"b", "a"}) {
		t.Error("expected order-insensitive equality")
	}
	if sameKeySet(existing, []string{"a"}) {
		t.Error("expected different lengths to differ")
	}
	if sameKeySet(existing, []string{"a", "c"}) {
		t.Error("expected different keys to differ")
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
	ps := buildPushSecret(testSecretName, testNamespace, kvNamespace(testNamespace), "vault-store",
		corev1.SecretTypeOpaque, []string{"a", "b"})

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
	data := spec["data"].([]map[string]any)
	if len(data) != 2 {
		t.Fatalf("data length = %d", len(data))
	}
	for i, want := range []string{"a", "b"} {
		match := data[i]["match"].(map[string]any)
		if match["secretKey"] != want {
			t.Errorf("data[%d].secretKey = %v", i, match["secretKey"])
		}
		remoteRef := match["remoteRef"].(map[string]any)
		if remoteRef["remoteKey"] != "secret/ns1/generic/my-secret" {
			t.Errorf("data[%d].remoteKey = %v", i, remoteRef["remoteKey"])
		}
		if remoteRef["property"] != want {
			t.Errorf("data[%d].property = %v", i, remoteRef["property"])
		}
	}
}

func TestBuildSecretReference(t *testing.T) {
	ref := buildSecretReference(testNamespace, testSecretName, corev1.SecretTypeBasicAuth,
		openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
		[]string{"password", "username"})

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

	svc := &secretService{k8sClient: k8sClient, planeClientProvider: mockProvider, logger: newTestLogger()}

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

	svc := &secretService{k8sClient: k8sClient, planeClientProvider: mockProvider, logger: newTestLogger()}

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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	_, err := svc.resolvePlane(context.Background(), testNamespace, planeKindWorkflowPlane, "wp1")
	if !errors.Is(err, ErrSecretStoreNotConfigured) {
		t.Errorf("expected ErrSecretStoreNotConfigured, got %v", err)
	}
}

func TestResolvePlane_InvalidKind(t *testing.T) {
	svc := &secretService{logger: newTestLogger()}
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
	svc := &secretService{logger: newTestLogger()}

	if err := svc.ensureNamespaceExists(context.Background(), k8sClient, "openchoreo-kv-ns1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureNamespaceExists_CreatesNew(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{logger: newTestLogger()}

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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

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
				Data:        map[string]string{"username": "u"}, // missing password
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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

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

// --- UpdateSecret error paths ---

func TestUpdateSecret_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, "missing", &UpdateSecretParams{
		Data: map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestUpdateSecret_NoTargetPlane(t *testing.T) {
	scheme := newTestScheme(t)
	legacy := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy", Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			Template: openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(legacy).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, "legacy", &UpdateSecretParams{
		Data: map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestUpdateSecret_ValidationError(t *testing.T) {
	scheme := newTestScheme(t)
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeBasicAuth},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	// basic-auth requires password
	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"username": "u"},
	})
	if !isValidationError(err) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestUpdateSecret_PlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "missing"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	_, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"k": "v"},
	})
	if !errors.Is(err, ErrPlaneNotFound) {
		t.Errorf("expected ErrPlaneNotFound, got %v", err)
	}
}

// --- DeleteSecret error paths ---

func TestDeleteSecret_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

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
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	err := svc.DeleteSecret(context.Background(), testNamespace, "legacy")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestDeleteSecret_PlaneNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "missing"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ref).Build()
	svc := &secretService{k8sClient: k8sClient, logger: newTestLogger()}

	err := svc.DeleteSecret(context.Background(), testNamespace, testSecretName)
	if !errors.Is(err, ErrPlaneNotFound) {
		t.Errorf("expected ErrPlaneNotFound, got %v", err)
	}
}

// --- happy paths via interceptor (SSA Patch + unstructured PushSecret) ---

// targetPlaneClient builds a fake client that:
//   - swallows SSA Patch calls (records count)
//   - returns NotFound for unstructured (PushSecret) Delete
//
// Real Delete on typed objects still goes through the underlying tracker.
type patchRec struct {
	count int
}

func newTargetPlaneClient(t *testing.T, scheme *runtime.Scheme, objs []client.Object, rec *patchRec) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(_ context.Context, _ client.WithWatch, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				if rec != nil {
					rec.count++
				}
				return nil
			},
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				// PushSecret is an unstructured external CRD; surface NotFound for it
				// so the service treats it as already-gone.
				if obj.GetObjectKind().GroupVersionKind().Group == "external-secrets.io" {
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

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger()}

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
	if len(info.Keys) != 2 {
		t.Errorf("info.Keys = %v", info.Keys)
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

func TestUpdateSecret_Success_SameKeys(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace, ResourceVersion: "1"},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeBasicAuth},
			Data: []openchoreov1alpha1.SecretDataSource{
				{SecretKey: "password", RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/ns1/basic-auth/my-secret", Property: "password"}},
				{SecretKey: "username", RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/ns1/basic-auth/my-secret", Property: "username"}},
			},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	rec := &patchRec{}
	targetClient := newTargetPlaneClient(t, scheme, nil, rec)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger()}

	info, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"password": "newP", "username": "u"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Keys) != 2 {
		t.Errorf("Keys = %v", info.Keys)
	}
	// Same keys → 2 patches (Secret + PushSecret). PushSecret is re-applied
	// on every Update so its sync-trigger annotation forces ESO to push the
	// new K8s Secret values to the external store immediately.
	if rec.count != 2 {
		t.Errorf("expected 2 SSA patches (Secret + PushSecret), got %d", rec.count)
	}
}

func TestUpdateSecret_Success_KeysChanged(t *testing.T) {
	scheme := newTestScheme(t)
	wp := &openchoreov1alpha1.WorkflowPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: testNamespace},
		Spec: openchoreov1alpha1.WorkflowPlaneSpec{
			SecretStoreRef: &openchoreov1alpha1.SecretStoreRef{Name: "store"},
		},
	}
	ref := &openchoreov1alpha1.SecretReference{
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace, ResourceVersion: "1"},
		Spec: openchoreov1alpha1.SecretReferenceSpec{
			TargetPlane: &openchoreov1alpha1.TargetPlaneRef{Kind: planeKindWorkflowPlane, Name: "wp1"},
			Template:    openchoreov1alpha1.SecretTemplate{Type: corev1.SecretTypeOpaque},
			Data: []openchoreov1alpha1.SecretDataSource{
				{SecretKey: "k1", RemoteRef: openchoreov1alpha1.RemoteReference{Key: "secret/ns1/generic/my-secret", Property: "k1"}},
			},
		},
	}
	cpClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wp, ref).Build()

	rec := &patchRec{}
	targetClient := newTargetPlaneClient(t, scheme, nil, rec)

	mockProvider := k8sMocks.NewMockPlaneClientProvider(t)
	mockProvider.EXPECT().WorkflowPlaneClient(wp).Return(targetClient, nil).Once()

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger()}

	info, err := svc.UpdateSecret(context.Background(), testNamespace, testSecretName, &UpdateSecretParams{
		Data: map[string]string{"k1": "v1", "k2": "v2"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(info.Keys) != 2 {
		t.Errorf("Keys = %v", info.Keys)
	}
	// Keys changed → 2 patches (Secret + PushSecret) and 1 SecretReference Update.
	if rec.count != 2 {
		t.Errorf("expected 2 SSA patches (Secret + PushSecret), got %d", rec.count)
	}

	got := &openchoreov1alpha1.SecretReference{}
	if err := cpClient.Get(context.Background(),
		client.ObjectKey{Name: testSecretName, Namespace: testNamespace}, got); err != nil {
		t.Fatalf("SecretReference vanished: %v", err)
	}
	if len(got.Spec.Data) != 2 {
		t.Errorf("spec.data length = %d", len(got.Spec.Data))
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
		ObjectMeta: metav1.ObjectMeta{Name: testSecretName, Namespace: testNamespace},
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

	svc := &secretService{k8sClient: cpClient, planeClientProvider: mockProvider, logger: newTestLogger()}

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
