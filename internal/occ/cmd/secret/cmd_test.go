// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

const (
	testUsername    = "admin"
	testNewPassword = "new"
)

func mockFactory(mc *mocks.MockInterface) client.NewClientFunc {
	return func() (client.Interface, error) {
		return mc, nil
	}
}

func errFactory(msg string) client.NewClientFunc {
	return func() (client.Interface, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

// --- root cmd structure ---

func TestNewSecretCmd_Use(t *testing.T) {
	cmd := NewSecretCmd(errFactory("unused"))
	assert.Equal(t, "secret", cmd.Use)
	assert.Contains(t, cmd.Aliases, "secrets")
}

func TestNewSecretCmd_Subcommands(t *testing.T) {
	cmd := NewSecretCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"list", "get", "delete", "create", "update"}, names)
}

func TestCreateCmd_Subcommands(t *testing.T) {
	cmd := newCreateCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"generic", "docker-registry", "tls"}, names)
}

// --- list ---

func TestListCmd_FactoryError(t *testing.T) {
	cmd := newListCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, nil)
	assert.EqualError(t, err, "factory failed")
}

func TestListCmd_Success(t *testing.T) {
	const ns = "acme-corp"
	mc := mocks.NewMockInterface(t)
	tp := &gen.TargetPlaneRef{Kind: gen.TargetPlaneRefKindDataPlane, Name: "dp-prod"}
	mc.EXPECT().ListSecrets(mock.Anything, ns, mock.Anything).Return(&gen.ListSecretsResponse{
		Items: []gen.Secret{
			{Metadata: gen.ObjectMeta{Name: "my-secret"}, Type: "Opaque"},
		},
		Pagination: gen.Pagination{},
	}, nil)
	mc.EXPECT().ListSecretReferences(mock.Anything, ns, mock.Anything).Return(&gen.SecretReferenceList{
		Items: []gen.SecretReference{
			{Metadata: gen.ObjectMeta{Name: "my-secret"}, Spec: &gen.SecretReferenceSpec{TargetPlane: tp}},
		},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", ns))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "my-secret")
	assert.Contains(t, out, "DataPlane/dp-prod")
}

// --- get ---

func TestGetCmd_MissingArg(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_NAME")
}

func TestGetCmd_FactoryError(t *testing.T) {
	cmd := newGetCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-secret"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetSecret(mock.Anything, mock.Anything, "my-secret").Return(
		&gen.Secret{Metadata: gen.ObjectMeta{Name: "my-secret"}, Type: "Opaque"}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-secret"}))
	})
	assert.Contains(t, out, "name: my-secret")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-secret"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteSecret(mock.Anything, "acme-corp", "my-secret").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-secret"}))
	})
	assert.Contains(t, out, "deleted")
}

// --- update ---

func TestUpdateCmd_MissingArg(t *testing.T) {
	cmd := newUpdateCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_NAME")
}

func TestUpdateCmd_FactoryError(t *testing.T) {
	cmd := newUpdateCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-secret"})
	assert.EqualError(t, err, "factory failed")
}

func TestUpdateCmd_Merge_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	existing := map[string][]byte{"username": []byte(testUsername), "password": []byte("old")}
	mc.EXPECT().GetSecret(mock.Anything, "acme-corp", "db-creds").Return(
		&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}, Type: "Opaque", Data: &existing}, nil,
	)
	mc.EXPECT().UpdateSecret(mock.Anything, "acme-corp", "db-creds", mock.MatchedBy(func(req gen.UpdateSecretRequest) bool {
		return req.Data["username"] == testUsername && req.Data["password"] == testNewPassword && len(req.Data) == 2
	})).Return(&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}}, nil)

	cmd := newUpdateCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	require.NoError(t, cmd.Flags().Set("from-literal", "password="+testNewPassword))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"db-creds"}))
	})
	assert.Contains(t, out, "updated")
}

func TestUpdateCmd_Replace_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	existing := map[string][]byte{"old": []byte("v")}
	mc.EXPECT().GetSecret(mock.Anything, "acme-corp", "db-creds").Return(
		&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}, Type: "Opaque", Data: &existing}, nil,
	)
	mc.EXPECT().UpdateSecret(mock.Anything, "acme-corp", "db-creds", mock.MatchedBy(func(req gen.UpdateSecretRequest) bool {
		return len(req.Data) == 1 && req.Data["password"] == testNewPassword
	})).Return(&gen.Secret{Metadata: gen.ObjectMeta{Name: "db-creds"}}, nil)

	cmd := newUpdateCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	require.NoError(t, cmd.Flags().Set("replace", "true"))
	require.NoError(t, cmd.Flags().Set("from-literal", "password="+testNewPassword))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"db-creds"}))
	})
	assert.Contains(t, out, "updated")
}

// --- create generic ---

func TestCreateGenericCmd_MissingArg(t *testing.T) {
	cmd := newCreateGenericCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SECRET_NAME")
}

func TestCreateGenericCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "acme-corp", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		return req.SecretName == "db-creds" &&
			req.SecretType == gen.SecretTypeOpaque &&
			req.TargetPlane.Kind == gen.TargetPlaneRefKindDataPlane &&
			req.TargetPlane.Name == "dp-prod" &&
			req.Data["username"] == testUsername &&
			req.Data["password"] == "s3cret"
	})).Return(&gen.Secret{}, nil)

	cmd := newCreateGenericCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	require.NoError(t, cmd.Flags().Set("target-plane", "DataPlane/dp-prod"))
	require.NoError(t, cmd.Flags().Set("from-literal", "username="+testUsername))
	require.NoError(t, cmd.Flags().Set("from-literal", "password=s3cret"))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"db-creds"}))
	})
	assert.Contains(t, out, "created")
}

// --- create docker-registry ---

func TestCreateDockerRegistryCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "acme-corp", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		_, ok := req.Data[".dockerconfigjson"]
		return req.SecretType == gen.SecretTypeKubernetesIodockerconfigjson && ok
	})).Return(&gen.Secret{}, nil)

	cmd := newCreateDockerRegistryCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	require.NoError(t, cmd.Flags().Set("target-plane", "DataPlane/dp-prod"))
	require.NoError(t, cmd.Flags().Set("docker-server", "https://index.docker.io/v1/"))
	require.NoError(t, cmd.Flags().Set("docker-username", "jdoe"))
	require.NoError(t, cmd.Flags().Set("docker-password", "hunter2"))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"regcred"}))
	})
	assert.Contains(t, out, "created")
}

// --- create tls ---

func TestCreateTLSCmd_Success(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	require.NoError(t, os.WriteFile(certPath, []byte("CERT"), 0o600))
	require.NoError(t, os.WriteFile(keyPath, []byte("KEY"), 0o600))

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().CreateSecret(mock.Anything, "acme-corp", mock.MatchedBy(func(req gen.CreateSecretRequest) bool {
		return req.SecretType == gen.SecretTypeKubernetesIotls &&
			req.Data["tls.crt"] == "CERT" &&
			req.Data["tls.key"] == "KEY"
	})).Return(&gen.Secret{}, nil)

	cmd := newCreateTLSCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	require.NoError(t, cmd.Flags().Set("target-plane", "DataPlane/dp-prod"))
	require.NoError(t, cmd.Flags().Set("cert", certPath))
	require.NoError(t, cmd.Flags().Set("key", keyPath))

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-tls"}))
	})
	assert.Contains(t, out, "created")
}
