// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func dataPlane(name string) *gen.TargetPlaneRef {
	return &gen.TargetPlaneRef{Kind: gen.TargetPlaneRefKindDataPlane, Name: name}
}

// --- printList ---

func TestPrintList_Empty(t *testing.T) {
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(nil, nil))
	})
	assert.Contains(t, out, "No secrets found")
}

func TestPrintList_WithItems(t *testing.T) {
	now := time.Now()
	items := []gen.Secret{
		{
			Metadata: gen.ObjectMeta{Name: "tls-1", CreationTimestamp: &now},
			Type:     "kubernetes.io/tls",
		},
	}
	targets := map[string]string{"tls-1": "DataPlane/dp-prod"}
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, printList(items, targets))
	})
	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "TYPE")
	assert.Contains(t, out, "TARGET PLANE")
	assert.Contains(t, out, "tls-1")
	assert.Contains(t, out, "kubernetes.io/tls")
	assert.Contains(t, out, "DataPlane/dp-prod")
}

// --- List ---

func TestList_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	s := New(mc)
	err := s.List(ListParams{Namespace: ""})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestList_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListSecrets(mock.Anything, "org-a", mock.Anything).Return(&gen.ListSecretsResponse{
		Items: []gen.Secret{
			{Metadata: gen.ObjectMeta{Name: "api-key"}, Type: "Opaque"},
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().ListSecretReferences(mock.Anything, "org-a", mock.Anything).Return(&gen.SecretReferenceList{
		Items: []gen.SecretReference{
			{Metadata: gen.ObjectMeta{Name: "api-key"}, Spec: &gen.SecretReferenceSpec{TargetPlane: dataPlane("dp")}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).List(ListParams{Namespace: "org-a"}))
	})
	assert.Contains(t, out, "api-key")
	assert.Contains(t, out, "Opaque")
	assert.Contains(t, out, "DataPlane/dp")
}

// --- Get ---

func TestGet_ValidationError_NoName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Get(GetParams{Namespace: "ns"})
	assert.ErrorContains(t, err, "Missing required parameter: --name")
}

func TestGet_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, "ns", "x").Return(
		&gen.Secret{Metadata: gen.ObjectMeta{Name: "x"}, Type: "Opaque"}, nil,
	)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Get(GetParams{Namespace: "ns", SecretName: "x"}))
	})
	assert.Contains(t, out, "name: x")
}

// --- Delete ---

func TestDelete_ValidationError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Delete(DeleteParams{Namespace: "", SecretName: "x"})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestDelete_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteSecret(mock.Anything, "ns", "x").Return(fmt.Errorf("boom"))
	err := New(mc).Delete(DeleteParams{Namespace: "ns", SecretName: "x"})
	assert.EqualError(t, err, "boom")
}

func TestDelete_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteSecret(mock.Anything, "ns", "x").Return(nil)
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Delete(DeleteParams{Namespace: "ns", SecretName: "x"}))
	})
	assert.Contains(t, out, "Secret 'x' deleted")
}

// --- CreateGeneric ---

func TestCreateGeneric_RequiresData(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
	}, "")
	assert.ErrorContains(t, err, "at least one of --from-literal")
}

func TestCreateGeneric_InvalidTargetPlane(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "bogus",
		FromLiteral: []string{"k=v"},
	}, "")
	assert.ErrorContains(t, err, "invalid --target-plane")
}

func TestCreateGeneric_OpaqueByDefault(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		return req.SecretType == gen.SecretTypeOpaque && req.Data["k"] == "v"
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
		FromLiteral: []string{"k=v"},
	}, ""))
}

func TestCreateGeneric_TypeOverride(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		return req.SecretType == gen.SecretTypeKubernetesIobasicAuth
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
		FromLiteral: []string{"username=admin", "password=s3"},
	}, "kubernetes.io/basic-auth"))
}

// --- CreateDockerRegistry ---

func TestCreateDockerRegistry_BuildsConfigJSON(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	var captured gen.CreateSecretRequest
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.Anything).Run(func(_ context.Context, _ string, req gen.CreateSecretRequest) {
		captured = req
	}).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateDockerRegistry(CreateInput{
		Namespace:   "ns",
		SecretName:  "regcred",
		TargetPlane: "DataPlane/dp",
	}, "https://reg.example/v1/", "jdoe", "hunter2", "jdoe@example.com"))

	assert.Equal(t, gen.SecretTypeKubernetesIodockerconfigjson, captured.SecretType)
	raw, ok := captured.Data[".dockerconfigjson"]
	require.True(t, ok)

	var parsed struct {
		Auths map[string]struct {
			Username, Password, Email, Auth string
		} `json:"auths"`
	}
	require.NoError(t, json.Unmarshal([]byte(raw), &parsed))
	entry, ok := parsed.Auths["https://reg.example/v1/"]
	require.True(t, ok)
	assert.Equal(t, "jdoe", entry.Username)
	assert.Equal(t, "hunter2", entry.Password)
	assert.Equal(t, "jdoe@example.com", entry.Email)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("jdoe:hunter2")), entry.Auth)
}

func TestCreateDockerRegistry_MissingServer(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).CreateDockerRegistry(CreateInput{
		Namespace:   "ns",
		SecretName:  "regcred",
		TargetPlane: "DataPlane/dp",
	}, "", "jdoe", "hunter2", "")
	assert.ErrorContains(t, err, "--docker-server")
}

// --- CreateTLS ---

func TestCreateTLS_Success(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "tls.crt")
	key := filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(cert, []byte("C"), 0o600))
	require.NoError(t, os.WriteFile(key, []byte("K"), 0o600))

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		return req.SecretType == gen.SecretTypeKubernetesIotls &&
			req.Data["tls.crt"] == "C" && req.Data["tls.key"] == "K"
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateTLS(CreateInput{
		Namespace:   "ns",
		SecretName:  "tls",
		TargetPlane: "DataPlane/dp",
	}, cert, key))
}

func TestCreateTLS_MissingFile(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).CreateTLS(CreateInput{
		Namespace:   "ns",
		SecretName:  "tls",
		TargetPlane: "DataPlane/dp",
	}, "/no/such/cert", "/no/such/key")
	assert.ErrorContains(t, err, "read --cert")
}

// --- parseTargetPlane ---

func TestParseTargetPlane(t *testing.T) {
	tp, err := parseTargetPlane("DataPlane/dp-prod")
	require.NoError(t, err)
	assert.Equal(t, gen.TargetPlaneRefKindDataPlane, tp.Kind)
	assert.Equal(t, "dp-prod", tp.Name)

	_, err = parseTargetPlane("DataPlane")
	assert.Error(t, err)

	_, err = parseTargetPlane("Bogus/x")
	assert.ErrorContains(t, err, "invalid --target-plane kind")
}

// --- collectData ---

func TestCollectData_AllSources(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "license.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("file-body"), 0o600))

	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("# comment\nFOO=bar\nBAZ=qux\n"), 0o600))

	data, err := collectData(
		[]string{"k1=v1"},
		[]string{"named=" + filePath, filePath},
		[]string{envPath},
	)
	require.NoError(t, err)
	assert.Equal(t, "v1", data["k1"])
	assert.Equal(t, "file-body", data["named"])
	assert.Equal(t, "file-body", data["license.txt"])
	assert.Equal(t, "bar", data["FOO"])
	assert.Equal(t, "qux", data["BAZ"])
}

func TestCollectData_InvalidLiteral(t *testing.T) {
	_, err := collectData([]string{"nobueno"}, nil, nil)
	assert.ErrorContains(t, err, "invalid --from-literal")
}

func TestCollectData_InvalidEnvFileLine(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("NOTKV\n"), 0o600))
	_, err := collectData(nil, nil, []string{envPath})
	assert.ErrorContains(t, err, "expected KEY=VALUE")
}
