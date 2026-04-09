// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client"
	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
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

// --- NewComponentCmd structure ---

func TestNewComponentCmd_Use(t *testing.T) {
	cmd := NewComponentCmd(errFactory("unused"))
	assert.Equal(t, "component", cmd.Use)
	assert.Contains(t, cmd.Aliases, "comp")
	assert.Contains(t, cmd.Aliases, "components")
}

func TestNewComponentCmd_Subcommands(t *testing.T) {
	cmd := NewComponentCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "get")
	assert.Contains(t, names, "delete")
	assert.Contains(t, names, "scaffold")
	assert.Contains(t, names, "deploy")
	assert.Contains(t, names, "logs")
	assert.Contains(t, names, "workflow")
	assert.Contains(t, names, "workflowrun")
}

// --- list ---

func TestListCmd_FactoryError(t *testing.T) {
	cmd := newListCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, nil)
	assert.EqualError(t, err, "factory failed")
}

func TestListCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponents(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&gen.ComponentList{
		Items:      []gen.Component{{Metadata: gen.ObjectMeta{Name: "my-component"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "my-component")
}

// --- get ---

func TestGetCmd_MissingArg(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestGetCmd_TooManyArgs(t *testing.T) {
	cmd := newGetCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{"a", "b"})
	assert.EqualError(t, err, "accepts 1 arg(s), received 2")
}

func TestGetCmd_FactoryError(t *testing.T) {
	cmd := newGetCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponent(mock.Anything, mock.Anything, "my-component").Return(
		&gen.Component{Metadata: gen.ObjectMeta{Name: "my-component"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-component"}))
	})
	assert.Contains(t, out, "my-component")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "COMPONENT_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponent(mock.Anything, mock.Anything, "my-component").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-component"}))
	})
	assert.Contains(t, out, "deleted")
}

// --- scaffold ---

func TestScaffoldCmd_MissingArg(t *testing.T) {
	cmd := newScaffoldCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

// --- deploy ---

func TestDeployCmd_MissingArg(t *testing.T) {
	cmd := newDeployCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestDeployCmd_FactoryError(t *testing.T) {
	cmd := newDeployCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

// --- logs ---

func TestLogsCmd_MissingArg(t *testing.T) {
	cmd := newLogsCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestLogsCmd_FactoryError(t *testing.T) {
	cmd := newLogsCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

func TestLogsCmd_Flags(t *testing.T) {
	cmd := newLogsCmd(errFactory("unused"))
	for _, name := range []string{"namespace", "project", "env", "follow", "since", "tail"} {
		assert.NotNil(t, cmd.Flags().Lookup(name), "expected flag %q", name)
	}
}

// --- workflow subcommands ---

func TestWorkflowCmd_Subcommands(t *testing.T) {
	cmd := newWorkflowCmd(errFactory("unused"))
	assert.Equal(t, "workflow", cmd.Use)
	assert.Contains(t, cmd.Aliases, "wf")

	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "run")
	assert.Contains(t, names, "logs")
}

func TestStartWorkflowCmd_MissingArg(t *testing.T) {
	cmd := newStartWorkflowCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestStartWorkflowCmd_FactoryError(t *testing.T) {
	cmd := newStartWorkflowCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

func TestWorkflowLogsCmd_MissingArg(t *testing.T) {
	cmd := newWorkflowLogsCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestWorkflowLogsCmd_FactoryError(t *testing.T) {
	cmd := newWorkflowLogsCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

// --- workflowrun subcommands ---

func TestWorkflowRunCmd_Subcommands(t *testing.T) {
	cmd := newWorkflowRunCmd(errFactory("unused"))
	assert.Equal(t, "workflowrun", cmd.Use)
	assert.Contains(t, cmd.Aliases, "wr")

	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.Contains(t, names, "list")
	assert.Contains(t, names, "logs")
}

func TestListWorkflowRunCmd_MissingArg(t *testing.T) {
	cmd := newListWorkflowRunCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestListWorkflowRunCmd_FactoryError(t *testing.T) {
	cmd := newListWorkflowRunCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

func TestWorkflowRunLogsCmd_MissingArg(t *testing.T) {
	cmd := newWorkflowRunLogsCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")
}

func TestWorkflowRunLogsCmd_FactoryError(t *testing.T) {
	cmd := newWorkflowRunLogsCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-component"})
	assert.EqualError(t, err, "factory failed")
}

// --- parseCSV ---

func TestParseCSV(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want []string
	}{
		{name: "empty value", val: "", want: nil},
		{name: "single value", val: "storage", want: []string{"storage"}},
		{name: "multiple values", val: "storage,ingress,logging", want: []string{"storage", "ingress", "logging"}},
		{name: "with spaces", val: " storage , ingress ", want: []string{"storage", "ingress"}},
		{name: "trailing comma", val: "storage,", want: []string{"storage"}},
		{name: "only commas", val: ",,", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("traits", tt.val, "")
			got := parseCSV(cmd, "traits")
			assert.Equal(t, tt.want, got)
		})
	}
}
