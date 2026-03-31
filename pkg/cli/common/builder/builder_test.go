// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package builder

import (
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/pkg/cli/common/constants"
	"github.com/openchoreo/openchoreo/pkg/cli/flags"
)

func TestFlagGetterGetString(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "resource name")
	require.NoError(t, cmd.Flags().Set("name", "my-project"))

	fg := &FlagGetter{cmd: cmd}
	assert.Equal(t, "my-project", fg.GetString(flags.Flag{Name: "name"}))
}

func TestFlagGetterGetStringDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("name", "", "resource name")

	fg := &FlagGetter{cmd: cmd}
	assert.Equal(t, "", fg.GetString(flags.Flag{Name: "name"}))
}

func TestFlagGetterGetBool(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("follow", false, "follow logs")
	require.NoError(t, cmd.Flags().Set("follow", "true"))

	fg := &FlagGetter{cmd: cmd}
	assert.True(t, fg.GetBool(flags.Flag{Name: "follow"}))
}

func TestFlagGetterGetBoolDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("follow", false, "follow logs")

	fg := &FlagGetter{cmd: cmd}
	assert.False(t, fg.GetBool(flags.Flag{Name: "follow"}))
}

func TestFlagGetterGetInt(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("tail", 0, "number of lines")
	require.NoError(t, cmd.Flags().Set("tail", "50"))

	fg := &FlagGetter{cmd: cmd}
	assert.Equal(t, 50, fg.GetInt(flags.Flag{Name: "tail"}))
}

func TestFlagGetterGetIntDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("tail", 0, "number of lines")

	fg := &FlagGetter{cmd: cmd}
	assert.Equal(t, 0, fg.GetInt(flags.Flag{Name: "tail"}))
}

func TestFlagGetterGetStringArray(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("set", nil, "set values")
	require.NoError(t, cmd.Flags().Set("set", "a=1"))
	require.NoError(t, cmd.Flags().Set("set", "b=2"))

	fg := &FlagGetter{cmd: cmd}
	assert.Equal(t, []string{"a=1", "b=2"}, fg.GetStringArray(flags.Flag{Name: "set"}))
}

func TestFlagGetterGetStringArrayDefault(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringArray("set", nil, "set values")

	fg := &FlagGetter{cmd: cmd}
	assert.Empty(t, fg.GetStringArray(flags.Flag{Name: "set"}))
}

func TestFlagGetterGetArgs(t *testing.T) {
	fg := &FlagGetter{args: []string{"arg1", "arg2"}}
	assert.Equal(t, []string{"arg1", "arg2"}, fg.GetArgs())
}

func TestFlagGetterGetArgsEmpty(t *testing.T) {
	fg := &FlagGetter{args: []string{}}
	assert.Empty(t, fg.GetArgs())
}

func TestFlagGetterGetCommand(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	fg := &FlagGetter{cmd: cmd}
	assert.Same(t, cmd, fg.GetCommand())
}

func TestCommandBuilderBuild(t *testing.T) {
	var capturedFG *FlagGetter
	b := &CommandBuilder{
		Command: constants.Command{
			Use:     "create",
			Aliases: []string{"new"},
			Short:   "Create a resource",
			Long:    "Create a new resource in the system",
			Example: "occ create project --name foo",
		},
		Flags: []flags.Flag{
			{Name: "name", Usage: "resource name"},
			{Name: "follow", Type: "bool", Usage: "follow"},
		},
		RunE: func(fg *FlagGetter) error {
			capturedFG = fg
			return nil
		},
	}

	cmd := b.Build()

	assert.Equal(t, "create", cmd.Use)
	assert.Equal(t, []string{"new"}, cmd.Aliases)
	assert.Equal(t, "Create a resource", cmd.Short)
	assert.Equal(t, "Create a new resource in the system", cmd.Long)
	assert.Equal(t, "occ create project --name foo", cmd.Example)

	// Verify flags were added
	assert.NotNil(t, cmd.Flags().Lookup("name"))
	assert.NotNil(t, cmd.Flags().Lookup("follow"))

	// Execute the command and verify RunE receives a FlagGetter
	require.NoError(t, cmd.Flags().Set("name", "my-project"))
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
	require.NotNil(t, capturedFG)
	assert.Equal(t, "my-project", capturedFG.GetString(flags.Flag{Name: "name"}))
}

func TestCommandBuilderBuildRunEError(t *testing.T) {
	b := &CommandBuilder{
		Command: constants.Command{Use: "fail"},
		RunE: func(fg *FlagGetter) error {
			return fmt.Errorf("something went wrong")
		},
	}

	cmd := b.Build()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, "something went wrong", err.Error())
}

func TestCommandBuilderBuildWithPreRunE(t *testing.T) {
	preRunCalled := false
	b := &CommandBuilder{
		Command: constants.Command{Use: "test"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			preRunCalled = true
			return nil
		},
		RunE: func(fg *FlagGetter) error {
			return nil
		},
	}

	cmd := b.Build()
	cmd.SetArgs([]string{})
	require.NoError(t, cmd.Execute())
	assert.True(t, preRunCalled)
}

func TestCommandBuilderBuildWithPreRunEError(t *testing.T) {
	b := &CommandBuilder{
		Command: constants.Command{Use: "test"},
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("precondition failed")
		},
		RunE: func(fg *FlagGetter) error {
			t.Fatal("RunE should not be called when PreRunE fails")
			return nil
		},
	}

	cmd := b.Build()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Equal(t, "precondition failed", err.Error())
}

func TestCommandBuilderBuildPassesArgs(t *testing.T) {
	var receivedArgs []string
	b := &CommandBuilder{
		Command: constants.Command{
			Use: "test [NAME]",
		},
		RunE: func(fg *FlagGetter) error {
			receivedArgs = fg.GetArgs()
			return nil
		},
	}

	cmd := b.Build()
	cmd.SetArgs([]string{"my-resource"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, []string{"my-resource"}, receivedArgs)
}
