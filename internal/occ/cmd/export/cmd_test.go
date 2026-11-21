// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package export

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

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

func TestNewExportCmd_ReleaseHistoryFlags(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{name: "default", want: "plus binding-referenced releases"},
		{name: "all releases", args: []string{"--all-releases"}, want: "All releases (--all-releases)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := mockComponentClient(t, "", "")
			cmd := NewExportCmd(func() (client.Interface, error) { return c, nil })
			cmd.PreRunE = nil
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetArgs(append([]string{"-o", t.TempDir()}, tt.args...))
			require.NoError(t, cmd.Execute())
			assert.Contains(t, output.String(), tt.want)
		})
	}
}

func TestNewExportCmd_ShowCounts(t *testing.T) {
	for _, showCounts := range []bool{false, true} {
		name := "default"
		if showCounts {
			name = "show counts"
		}
		t.Run(name, func(t *testing.T) {
			c := mockComponentClient(t, "", "")
			cmd := NewExportCmd(func() (client.Interface, error) { return c, nil })
			cmd.PreRunE = nil
			var output bytes.Buffer
			cmd.SetOut(&output)
			args := []string{"-o", t.TempDir()}
			if showCounts {
				args = append(args, "--show-counts")
			}
			cmd.SetArgs(args)
			require.NoError(t, cmd.Execute())
			if showCounts {
				assert.Contains(t, output.String(), "Resources by kind:")
				assert.Regexp(t, `Component\s+1\n`, output.String())
				assert.Regexp(t, `Namespace\s+1\n`, output.String())
			} else {
				assert.NotContains(t, output.String(), "Resources by kind:")
			}
			assert.NotContains(t, output.String(), "Older referenced:")
			assert.NotContains(t, output.String(), "Older unreferenced:")
		})
	}
}

func TestNewExportCmd_RejectsInvalidReleaseHistoryBeforeFetching(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "removed months option", args: []string{"--release-months", "12"}},
		{name: "removed cutoff option", args: []string{"--releases-since", "2026-05-01T00:00:00Z"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "output")
			cmd := NewExportCmd(func() (client.Interface, error) {
				t.Fatal("invalid history options must be rejected before creating the client")
				return nil, nil
			})
			cmd.PreRunE = nil
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs(append([]string{"-o", root}, tt.args...))
			require.Error(t, cmd.Execute())
			assert.NoDirExists(t, root)
		})
	}
}

func TestPromptKeepPlaintext(t *testing.T) {
	finding := Withheld{
		Kind: "Component", Namespace: "acme", Project: testProjectName, Name: "api",
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
			got, err := promptKeepPlaintext(t.Context(), bufio.NewReader(strings.NewReader(test.input)), &output, finding)
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
	_, err := promptKeepPlaintext(t.Context(), bufio.NewReader(strings.NewReader("n\n")), &output, Withheld{
		Kind: KindClusterResourceType, Name: "nats", File: "platform-shared/resource-types/nats.yaml",
		Reasons: []string{".spec.password"},
	})
	require.NoError(t, err)
	assert.Contains(t, output.String(), "Resource: ClusterResourceType nats\n")
}

func TestPromptKeepPlaintext_ReadError(t *testing.T) {
	readErr := errors.New("input failed")
	var output bytes.Buffer
	_, err := promptKeepPlaintext(t.Context(), bufio.NewReader(iotest.ErrReader(readErr)), &output, Withheld{})
	require.ErrorIs(t, err, readErr)
}

func TestNewExportCmd_PlaintextChoices(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		kept        [3]bool // Namespace token, component password, and environment secret, in prompt order.
		prompts     int
		showDetails bool
	}{
		{name: "individual choices", input: "y\nn\ny\n", kept: [3]bool{true, false, true}, prompts: 3},
		{name: "keep all", input: "a\n", kept: [3]bool{true, true, true}, prompts: 1},
		{name: "remove all", input: "r\n", prompts: 1},
		{name: "keep remaining", input: "n\na\n", kept: [3]bool{false, true, true}, prompts: 2},
		{name: "remove remaining", input: "y\nr\n", kept: [3]bool{true, false, false}, prompts: 2},
		{name: "no input removes", input: "", prompts: 3},
		{name: "explicit credential details", input: "y\nn\ny\n", kept: [3]bool{true, false, true}, prompts: 3, showDetails: true},
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
			args := []string{"-o", root}
			if tt.showDetails {
				args = append(args, "--show-credential-details")
			}
			cmd.SetArgs(args)
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
			_, summary, found := strings.Cut(output.String(), "Plaintext credentials:\n")
			require.True(t, found)
			kept := 0
			for _, keep := range tt.kept {
				if keep {
					kept++
				}
			}
			assert.Contains(t, summary, fmt.Sprintf("%d kept, %d removed", kept, len(tt.kept)-kept))
			if tt.showDetails {
				assert.Regexp(t, `Action:\s+Kept`, summary)
				assert.Regexp(t, `Action:\s+Removed`, summary)
				assert.Contains(t, summary, "namespaces/acme/projects/shop/components/api/component.yaml")
				assert.Contains(t, summary, ".spec.parameters.dbPassword")
			} else {
				assert.NotContains(t, summary, "Action:")
				assert.NotContains(t, summary, "File:")
				assert.NotContains(t, summary, "Field:")
			}
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
				assert.NoFileExists(t, filepath.Join(root, "namespaces", "acme", "namespace.yaml"))
			} else {
				assert.NoDirExists(t, root)
			}
		})
	}
}

func TestNewExportCmd_ContextCancellationDuringPrompt(t *testing.T) {
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
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			readStarted := make(chan struct{})
			readFinished := make(chan struct{})
			releaseInput := make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseInput) })
			var readOnce sync.Once
			lateChoice := strings.NewReader("a\n")
			input := promptReaderFunc(func(p []byte) (int, error) {
				readOnce.Do(func() { close(readStarted) })
				<-releaseInput
				n, err := lateChoice.Read(p)
				close(readFinished)
				return n, err
			})
			var output bytes.Buffer
			cmd := NewExportCmd(func() (client.Interface, error) { return c, nil })
			cmd.PreRunE = nil
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetIn(input)
			cmd.SetOut(&output)
			cmd.SetArgs([]string{"-o", root})
			finished := make(chan struct{})
			var executeErr error
			go func() {
				executeErr = cmd.ExecuteContext(ctx)
				close(finished)
			}()
			t.Cleanup(func() {
				cancel()
				release()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Error("export did not finish after releasing input")
				}
			})
			select {
			case <-readStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("export did not reach the credential input read")
			}
			cancel()
			select {
			case <-finished:
			case <-time.After(5 * time.Second):
				t.Fatal("context cancellation did not interrupt the credential prompt")
			}
			require.ErrorIs(t, executeErr, context.Canceled)
			release() // Supply a late keep-all response after the command has returned.
			select {
			case <-readFinished:
			case <-time.After(5 * time.Second):
				t.Fatal("pending read did not finish after supplying the late response")
			}
			assert.NotContains(t, output.String(), "Export completed")
			if existing {
				raw, err := os.ReadFile(filepath.Join(root, relativePath))
				require.NoError(t, err)
				assert.Equal(t, "existing manifest\n", string(raw))
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				assert.Len(t, entries, 1)
				assert.NoFileExists(t, filepath.Join(root, "namespaces", "acme", "namespace.yaml"))
			} else {
				assert.NoDirExists(t, root)
			}
		})
	}
}

type promptReaderFunc func([]byte) (int, error)

func (read promptReaderFunc) Read(p []byte) (int, error) {
	return read(p)
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
		Items: []gen.Project{{Metadata: gen.ObjectMeta{Name: testProjectName}}},
	}, nil)
	spec := componentSpec(testProjectName)
	parameters := map[string]any{
		"dbPassword": "component-secret-value",
		"password":   "{{ .password }}",
		"env": []any{map[string]any{
			"key": "CLIENT_SECRET", "value": "env-secret-value",
		}},
	}
	spec.Parameters = &parameters
	c.EXPECT().ListComponents(mock.Anything, "acme", testProjectName, mock.Anything).Return(&gen.ComponentList{
		Items: []gen.Component{{Metadata: gen.ObjectMeta{Name: "api"}, Spec: spec}},
	}, nil)
	c.EXPECT().ListWorkloads(mock.Anything, "acme", mock.Anything).Return(&gen.WorkloadList{}, nil)
	c.EXPECT().ListComponentReleases(mock.Anything, "acme", mock.Anything).Return(&gen.ComponentReleaseList{}, nil)
	c.EXPECT().ListReleaseBindings(mock.Anything, "acme", mock.Anything).Return(&gen.ReleaseBindingList{}, nil)
	return c
}
