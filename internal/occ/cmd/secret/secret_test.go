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

// --- Update ---

func bytesData(m map[string]string) *map[string][]byte {
	out := make(map[string][]byte, len(m))
	for k, v := range m {
		out[k] = []byte(v)
	}
	return &out
}

func TestUpdate_ValidationError_NoNamespace(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Update(UpdateInput{SecretName: "x", FromLiteral: []string{"k=v"}})
	assert.ErrorContains(t, err, "Missing required parameter: --namespace")
}

func TestUpdate_ValidationError_NoName(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Update(UpdateInput{Namespace: "ns", FromLiteral: []string{"k=v"}})
	assert.ErrorContains(t, err, "Missing required parameter: --name")
}

func TestUpdate_RequiresMutatingFlag(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Update(UpdateInput{Namespace: "ns", SecretName: "x"})
	assert.ErrorContains(t, err, "at least one of --from-literal")
}

// labelsPtr is a small helper for tests that populates the Metadata.Labels
// pointer on a fake gen.Secret response.
func labelsPtr(in map[string]string) *map[string]string {
	cp := make(map[string]string, len(in))
	for k, v := range in {
		cp[k] = v
	}
	return &cp
}

func TestUpdate_Replace_RequiresFrom(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	err := New(mc).Update(UpdateInput{Namespace: "ns", SecretName: "x", Replace: true})
	assert.ErrorContains(t, err, "at least one of --from-literal")
}

func TestUpdate_Merge_KeepsUnmentionedKeys(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, "ns", "db-creds").Return(&gen.Secret{
		Metadata: gen.ObjectMeta{
			Name:   "db-creds",
			Labels: labelsPtr(map[string]string{"openchoreo.dev/secret-type": categoryGeneric}),
		},
		Type: "Opaque",
		Data: bytesData(map[string]string{"username": testUsername, "password": "old"}),
	}, nil)
	mc.EXPECT().UpdateSecret(mock.Anything, "ns", "db-creds", mock.MatchedBy(func(req gen.UpdateSecretRequest) bool {
		if req.Data["username"] != testUsername || req.Data["password"] != testNewPassword || len(req.Data) != 2 {
			return false
		}
		// Category label must be carried forward so the API's full-replace
		// of labels does not drop it.
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == categoryGeneric
	})).Return(&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Update(UpdateInput{
			Namespace:   "ns",
			SecretName:  "db-creds",
			FromLiteral: []string{"password=" + testNewPassword},
		}))
	})
	assert.Contains(t, out, "Secret 'db-creds' updated")
}

func TestUpdate_Merge_NoCategoryLabelOnExisting(t *testing.T) {
	// If the existing secret has no secret-type label, the request sends
	// an empty labels map so other user labels are reset but no category
	// is invented.
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, "ns", "x").Return(&gen.Secret{
		Metadata: gen.ObjectMeta{Name: "x"},
		Type:     "Opaque",
		Data:     bytesData(map[string]string{"k": "v"}),
	}, nil)
	mc.EXPECT().UpdateSecret(mock.Anything, "ns", "x", mock.MatchedBy(func(req gen.UpdateSecretRequest) bool {
		if req.Labels == nil {
			return false
		}
		return len(*req.Labels) == 0
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).Update(UpdateInput{
		Namespace:   "ns",
		SecretName:  "x",
		FromLiteral: []string{"k=v2"},
	}))
}

func TestUpdate_Merge_GetError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, "ns", "x").Return(nil, fmt.Errorf("not found"))
	err := New(mc).Update(UpdateInput{
		Namespace:   "ns",
		SecretName:  "x",
		FromLiteral: []string{"k=v"},
	})
	assert.EqualError(t, err, "not found")
}

func TestUpdate_Replace_PrunesAndPreservesCategory(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	// Replace mode still GETs the secret so it can carry the existing
	// category label through the full-replace update.
	mc.EXPECT().GetSecret(mock.Anything, "ns", "db-creds").Return(&gen.Secret{
		Metadata: gen.ObjectMeta{
			Name:   "db-creds",
			Labels: labelsPtr(map[string]string{"openchoreo.dev/secret-type": "git-credentials"}),
		},
		Type: "Opaque",
		Data: bytesData(map[string]string{"old-key": "should-be-pruned"}),
	}, nil)
	mc.EXPECT().UpdateSecret(mock.Anything, "ns", "db-creds", mock.MatchedBy(func(req gen.UpdateSecretRequest) bool {
		if len(req.Data) != 2 || req.Data["username"] != testUsername || req.Data["password"] != testNewPassword {
			return false
		}
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == "git-credentials"
	})).Return(&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}}, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Update(UpdateInput{
			Namespace:   "ns",
			SecretName:  "db-creds",
			Replace:     true,
			FromLiteral: []string{"username=" + testUsername, "password=" + testNewPassword},
		}))
	})
	assert.Contains(t, out, "updated")
}

func TestUpdate_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, "ns", "x").Return(&gen.Secret{
		Metadata: gen.ObjectMeta{Name: "x"},
		Type:     "Opaque",
		Data:     bytesData(map[string]string{"a": "b"}),
	}, nil)
	mc.EXPECT().UpdateSecret(mock.Anything, "ns", "x", mock.Anything).Return(nil, fmt.Errorf("boom"))
	err := New(mc).Update(UpdateInput{
		Namespace:   "ns",
		SecretName:  "x",
		Replace:     true,
		FromLiteral: []string{"k=v"},
	})
	assert.EqualError(t, err, "boom")
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
		if req.SecretType != gen.SecretTypeOpaque || req.Data["k"] != "v" {
			return false
		}
		// Default category should always stamp the secret-type label.
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == categoryGeneric && len(*req.Labels) == 1
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
		FromLiteral: []string{"k=v"},
	}, ""))
}

func TestCreateGeneric_CategoryGitCredentials(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == "git-credentials"
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
		Category:    "git-credentials",
		FromLiteral: []string{"username=" + testUsername, "password=s3"},
	}, "kubernetes.io/basic-auth"))
}

func TestCreateGeneric_UnknownCategory(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	// No CreateSecret call expected: validation must fail first.
	err := New(mc).CreateGeneric(CreateInput{
		Namespace:   "ns",
		SecretName:  "n",
		TargetPlane: "DataPlane/dp",
		Category:    "bogus",
		FromLiteral: []string{"k=v"},
	}, "")
	assert.ErrorContains(t, err, "invalid --category")
}

func TestCreateDockerRegistry_CategoryDefault(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == categoryGeneric
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateDockerRegistry(CreateInput{
		Namespace:   "ns",
		SecretName:  "regcred",
		TargetPlane: "DataPlane/dp",
	}, "https://reg.example/v1/", "jdoe", "hunter2", ""))
}

func TestCreateTLS_CategoryDefault(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "tls.crt")
	key := filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(cert, []byte("C"), 0o600))
	require.NoError(t, os.WriteFile(key, []byte("K"), 0o600))

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "ns", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		if req.Labels == nil {
			return false
		}
		return (*req.Labels)["openchoreo.dev/secret-type"] == categoryGeneric
	})).Return(&gen.Secret{}, nil)

	require.NoError(t, New(mc).CreateTLS(CreateInput{
		Namespace:   "ns",
		SecretName:  "tls",
		TargetPlane: "DataPlane/dp",
	}, cert, key))
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
		FromLiteral: []string{"username=" + testUsername, "password=s3"},
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
