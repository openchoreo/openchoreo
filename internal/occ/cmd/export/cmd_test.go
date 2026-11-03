// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestNewExportCmd_RequiresOutputFlag(t *testing.T) {
	cmd := NewExportCmd(func() (client.Interface, error) { return nil, errors.New("unused") })
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.PreRunE = nil
	cmd.SetArgs(nil)

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output")
}

func TestRun_PrintsSummary(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	expectNoClusterResources(mockClient)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	var output bytes.Buffer

	require.NoError(t, Run(t.Context(), mockClient, &output, Params{OutputDir: t.TempDir()}))
	assert.Contains(t, output.String(), "0 resource(s)")
	assert.NotContains(t, output.String(), "Coverage is incomplete")
	assert.NotContains(t, output.String(), "ClusterWorkflowTemplate")
}

func TestRun_HidesDeniedResourceKind(t *testing.T) {
	mockClient := mocks.NewMockInterface(t)
	mockClient.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{}, nil)
	mockClient.EXPECT().ListClusterProjectTypes(mock.Anything, mock.Anything).Return(&gen.ClusterProjectTypeList{}, nil)
	mockClient.EXPECT().ListClusterResourceTypes(mock.Anything, mock.Anything).Return(&gen.ClusterResourceTypeList{}, nil)
	mockClient.EXPECT().ListClusterComponentTypes(mock.Anything, mock.Anything).Return(&gen.ClusterComponentTypeList{}, nil)
	mockClient.EXPECT().ListClusterTraits(mock.Anything, mock.Anything).Return(&gen.ClusterTraitList{}, nil)
	mockClient.EXPECT().ListClusterWorkflows(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowList{}, nil)
	mockClient.EXPECT().ListClusterDataPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterDataPlaneList{}, nil)
	mockClient.EXPECT().ListClusterWorkflowPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterWorkflowPlaneList{}, nil)
	mockClient.EXPECT().ListClusterObservabilityPlanes(mock.Anything, mock.Anything).Return(&gen.ClusterObservabilityPlaneList{}, nil)
	mockClient.EXPECT().ListClusterRoles(mock.Anything, mock.Anything).Return(nil, errors.New("You do not have permission to perform this operation"))

	var output bytes.Buffer
	err := Run(t.Context(), mockClient, &output, Params{OutputDir: t.TempDir()})
	require.EqualError(t, err, "your role does not have permission to perform this export operation!")
	assert.NotContains(t, err.Error(), "ClusterAuthzRole")
	assert.Empty(t, output.String(), "authorization failures should not print partial export details")
}

func TestPromptKeepPlaintext(t *testing.T) {
	finding := Withheld{
		Kind: "Component", Namespace: "acme", Project: "shop", Name: "api",
		File:    "namespaces/acme/projects/shop/components/api/component.yaml",
		Reasons: []string{".spec.parameters.dbPassword"},
	}
	tests := []struct {
		name  string
		input string
		want  plaintextChoice
		err   error
	}{
		{name: "yes keeps", input: "y\n", want: plaintextKeep},
		{name: "no strips", input: "n\n", want: plaintextRemove},
		{name: "keep all", input: "a\n", want: plaintextKeepAll},
		{name: "remove all", input: "r\n", want: plaintextRemoveAll},
		{name: "cancel", input: "q\n", err: errExportCanceled},
		{name: "empty defaults to strip", input: "\n", want: plaintextRemove},
		{name: "invalid then yes", input: "maybe\nyes\n", want: plaintextKeep},
		{name: "uppercase", input: " A \n", want: plaintextKeepAll},
		{name: "eof defaults to strip", input: "", want: plaintextRemove},
		{name: "choice without newline", input: "y", want: plaintextKeep},
		{name: "cancel without newline", input: "q", err: errExportCanceled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			got, err := promptKeepPlaintext(bufio.NewReader(strings.NewReader(test.input)), &output, finding)
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.want, got)
			assert.Contains(t, output.String(), "Possible plaintext credential\n  Resource: Component acme/shop/api\n  File: "+finding.File+"\n  Field: .spec.parameters.dbPassword\n  Reason: Literal value in a sensitive field\n")
			assert.Contains(t, output.String(), "Keep this value in the exported YAML?\n[y] Keep this\n[n] Remove this\n[a] Keep this and all remaining findings\n[r] Remove this and all remaining findings\n[q] Cancel export\nChoice [y/n/a/r/q]: ")
			assert.NotContains(t, output.String(), "secret-value")
		})
	}
}

func TestPromptKeepPlaintext_ClusterResourceName(t *testing.T) {
	var output bytes.Buffer
	_, err := promptKeepPlaintext(bufio.NewReader(strings.NewReader("n\n")), &output, Withheld{
		Kind: KindClusterResourceType, Name: "nats", File: "platform-shared/resource-types/nats.yaml",
		Reasons: []string{".spec.password"},
	})
	require.NoError(t, err)
	assert.Contains(t, output.String(), "Resource: ClusterResourceType nats\n")
}

func TestPromptKeepPlaintext_ReadError(t *testing.T) {
	readErr := errors.New("input failed")
	var output bytes.Buffer
	_, err := promptKeepPlaintext(bufio.NewReader(iotest.ErrReader(readErr)), &output, Withheld{})
	require.ErrorIs(t, err, readErr)
}

func TestNewExportCmd_PlaintextChoices(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		kept    [3]bool // Namespace token, component password, and environment secret, in prompt order.
		prompts int
	}{
		{name: "individual choices", input: "y\nn\ny\n", kept: [3]bool{true, false, true}, prompts: 3},
		{name: "keep all", input: "a\n", kept: [3]bool{true, true, true}, prompts: 1},
		{name: "remove all", input: "r\n", prompts: 1},
		{name: "keep remaining", input: "n\na\n", kept: [3]bool{false, true, true}, prompts: 2},
		{name: "remove remaining", input: "y\nr\n", kept: [3]bool{true, false, false}, prompts: 2},
		{name: "no input removes", input: "", prompts: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "export")
			c := mockCredentialPromptClient(t)
			var output bytes.Buffer
			cmd := NewExportCmd(func() (client.Interface, error) { return c, nil })
			cmd.PreRunE = nil
			cmd.SetIn(strings.NewReader(tt.input))
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"-o", root})
			require.NoError(t, cmd.Execute())

			component := readManifest(t, root, "namespaces/acme/projects/shop/components/api/component.yaml")
			parameters := component["spec"].(map[string]any)["parameters"].(map[string]any)
			env := parameters["env"].([]any)[0].(map[string]any)
			namespace := readManifest(t, root, "namespaces/acme/namespace.yaml")
			annotations := namespace["metadata"].(map[string]any)["annotations"].(map[string]any)
			_, passwordKept := parameters["dbPassword"]
			_, envKept := env["value"]
			_, namespaceKept := annotations["apiToken"]
			assert.Equal(t, tt.kept, [3]bool{namespaceKept, passwordKept, envKept})
			assert.Equal(t, "{{ .password }}", parameters["password"])
			assert.Equal(t, "CLIENT_SECRET", env["key"])
			assert.Equal(t, tt.prompts, strings.Count(output.String(), "Possible plaintext credential\n"))
			assert.NotContains(t, output.String(), "component-secret-value")
			assert.NotContains(t, output.String(), "env-secret-value")
			assert.NotContains(t, output.String(), "namespace-secret-value")
		})
	}
}

func TestNewExportCmd_CancelWritesNothing(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new output directory"
		if existing {
			name = "existing export"
		}
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "export")
			const relativePath = "namespaces/acme/projects/shop/components/api/component.yaml"
			if existing {
				_, err := writeFile(root, relativePath, []byte("existing manifest\n"))
				require.NoError(t, err)
			}
			c := mockCredentialPromptClient(t)
			var output bytes.Buffer
			cmd := NewExportCmd(func() (client.Interface, error) { return c, nil })
			cmd.PreRunE = nil
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetIn(strings.NewReader("y\nn\nq\n"))
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"-o", root})
			require.ErrorIs(t, cmd.Execute(), errExportCanceled)
			assert.NotContains(t, output.String(), "Exported ")
			if existing {
				raw, err := os.ReadFile(filepath.Join(root, relativePath))
				require.NoError(t, err)
				assert.Equal(t, "existing manifest\n", string(raw))
				assert.NoFileExists(t, filepath.Join(root, "namespaces/acme/namespace.yaml"))
			} else {
				assert.NoDirExists(t, root)
			}
		})
	}
}

func mockCredentialPromptClient(t *testing.T) *mocks.MockInterface {
	t.Helper()
	c := mocks.NewMockInterface(t)
	expectNoClusterResources(c)
	expectNoNamespacePlatformResources(c)
	expectNoProjectResources(c)
	annotations := map[string]string{"apiToken": "namespace-secret-value"}
	c.EXPECT().ListNamespaces(mock.Anything, mock.Anything).Return(&gen.NamespaceList{
		Items: []gen.Namespace{{Metadata: gen.ObjectMeta{Name: "acme", Annotations: &annotations}}},
	}, nil)
	c.EXPECT().ListProjects(mock.Anything, "acme", mock.Anything).Return(&gen.ProjectList{
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: "shop"}}},
	}, nil)
	spec := componentSpec("shop")
	parameters := map[string]any{
		"dbPassword": "component-secret-value",
		"password":   "{{ .password }}",
		"env": []any{map[string]any{
			"key": "CLIENT_SECRET", "value": "env-secret-value",
		}},
	}
	spec.Parameters = &parameters
	c.EXPECT().ListComponents(mock.Anything, "acme", "shop", mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{{Metadata: gen.ObjectMeta{Name: "api"}, Spec: spec}},
	}, nil)
	c.EXPECT().ListWorkloads(mock.Anything, "acme", mock.Anything).Return(&gen.WorkloadList{}, nil)
	c.EXPECT().ListComponentReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ComponentReleaseList{}, nil)
	c.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{}, nil)
	return c
}
