// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebindingrevision

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/cmd/bindingrevision"
	"github.com/openchoreo/openchoreo/internal/occ/cmd/config"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
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

// --- NewReleaseBindingRevisionCmd structure ---

func TestNewReleaseBindingRevisionCmd_Use(t *testing.T) {
	cmd := NewReleaseBindingRevisionCmd(errFactory("unused"))
	assert.Equal(t, "releasebindingrevision", cmd.Use)
	assert.Contains(t, cmd.Aliases, "rbr")
	assert.Contains(t, cmd.Aliases, "crbr")
	assert.Contains(t, cmd.Aliases, "releasebindingrevisions")
}

func TestNewReleaseBindingRevisionCmd_Subcommands(t *testing.T) {
	cmd := NewReleaseBindingRevisionCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"list", "get", "delete"}, names)
}

func TestSubcommandFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
	}{
		{name: "list", flags: []string{"namespace", "binding", "project", "component", "environment"}},
		{name: "get", flags: []string{"namespace", "binding", "revision", "output"}},
	}
	root := NewReleaseBindingRevisionCmd(errFactory("unused"))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, _, err := root.Find([]string{tt.name})
			require.NoError(t, err)
			for _, f := range tt.flags {
				assert.NotNil(t, sub.Flags().Lookup(f), "flag --%s", f)
			}
		})
	}
	get, _, _ := root.Find([]string{"get"})
	assert.Equal(t, "o", get.Flags().Lookup("output").Shorthand)
	assert.Equal(t, "table", get.Flags().Lookup("output").DefValue)
}

// --- factory errors ---

func TestCmds_FactoryError(t *testing.T) {
	for name, cmd := range map[string]func() error{
		"list": func() error { c := newListCmd(errFactory("factory failed")); return c.RunE(c, nil) },
		"get":  func() error { c := newGetCmd(errFactory("factory failed")); return c.RunE(c, nil) },
	} {
		assert.EqualError(t, cmd(), "factory failed", name)
	}
}

// --- end to end through cobra flags ---

func TestListCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", testNS))
	require.NoError(t, cmd.Flags().Set("binding", testBinding))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "KNOWN-GOOD")
	assert.Contains(t, out, "api-r3")
}

func TestGetCmd_YAMLViaShorthand(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	expectHistory(mc)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.ParseFlags([]string{"-n", testNS, "--binding", testBinding, "--revision", "1", "-o", "yaml"}))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "componentReleaseName: api-aaa")
}

// list's --project, --component and --resource are filters: they must not be
// filled in from the current context, which would silently narrow the list.
func TestListCmd_SkipsContextDefaults(t *testing.T) {
	cmd := newListCmd(errFactory("unused"))
	_, skip := cmd.Annotations[config.SkipContextDefaultsAnnotation]
	assert.True(t, skip)

	require.NoError(t, cmd.Flags().Set("namespace", "acme"))
	assert.Equal(t, "acme", bindingrevision.NamespaceOrContext(cmd), "an explicit --namespace wins")
}
