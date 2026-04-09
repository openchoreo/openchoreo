// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"fmt"
	"testing"

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

// --- NewComponentReleaseCmd structure ---

func TestNewComponentReleaseCmd_Use(t *testing.T) {
	cmd := NewComponentReleaseCmd(errFactory("unused"))
	assert.Equal(t, "componentrelease", cmd.Use)
	assert.Contains(t, cmd.Aliases, "cr")
	assert.Contains(t, cmd.Aliases, "componentreleases")
}

func TestNewComponentReleaseCmd_Subcommands(t *testing.T) {
	cmd := NewComponentReleaseCmd(errFactory("unused"))
	names := make([]string, 0, len(cmd.Commands()))
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	assert.ElementsMatch(t, []string{"generate", "list", "get", "delete"}, names)
}

// --- list ---

func TestListCmd_FactoryError(t *testing.T) {
	cmd := newListCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, nil)
	assert.EqualError(t, err, "factory failed")
}

func TestListCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().ListComponentReleases(mock.Anything, mock.Anything, mock.Anything).Return(&gen.ComponentReleaseList{
		Items:      []gen.ComponentRelease{{Metadata: gen.ObjectMeta{Name: "my-release"}}},
		Pagination: gen.Pagination{},
	}, nil)

	cmd := newListCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, nil))
	})
	assert.Contains(t, out, "my-release")
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
	err := cmd.RunE(cmd, []string{"my-release"})
	assert.EqualError(t, err, "factory failed")
}

func TestGetCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, mock.Anything, "my-release").Return(
		&gen.ComponentRelease{Metadata: gen.ObjectMeta{Name: "my-release"}}, nil,
	)

	cmd := newGetCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-release"}))
	})
	assert.Contains(t, out, "my-release")
}

// --- delete ---

func TestDeleteCmd_MissingArg(t *testing.T) {
	cmd := newDeleteCmd(errFactory("unused"))
	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "COMPONENT_RELEASE_NAME")
}

func TestDeleteCmd_FactoryError(t *testing.T) {
	cmd := newDeleteCmd(errFactory("factory failed"))
	err := cmd.RunE(cmd, []string{"my-release"})
	assert.EqualError(t, err, "factory failed")
}

func TestDeleteCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().DeleteComponentRelease(mock.Anything, mock.Anything, "my-release").Return(nil)

	cmd := newDeleteCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "acme-corp"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"my-release"}))
	})
	assert.Contains(t, out, "deleted")
}

// --- isFlagInArgs ---

func TestIsFlagInArgs_ExactMatch(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--all"})
	assert.True(t, isFlagInArgs("--all"))
	assert.False(t, isFlagInArgs("--project"))
}

func TestIsFlagInArgs_EqualsForm(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--project=my-proj"})
	assert.True(t, isFlagInArgs("--project"))
	assert.False(t, isFlagInArgs("--component"))
}

func TestIsFlagInArgs_NotPresent(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate"})
	assert.False(t, isFlagInArgs("--all"))
}

// --- generate validation ---

func TestGenerateCmd_NoScopeFlag(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate"})
	cmd := newGenerateCmd()
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "one of --all, --project, or --component must be specified")
}

func TestGenerateCmd_AllWithProject(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--all", "--project=my-proj"})
	cmd := newGenerateCmd()
	_ = cmd.Flags().Set("project", "my-proj")
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --project or --component")
}

func TestGenerateCmd_AllWithComponent(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--all", "--component=my-comp"})
	cmd := newGenerateCmd()
	_ = cmd.Flags().Set("component", "my-comp")
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --project or --component")
}

func TestGenerateCmd_AllWithName(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--all", "--name=my-name"})
	cmd := newGenerateCmd()
	_ = cmd.Flags().Set("name", "my-name")
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot be combined with --name")
}

func TestGenerateCmd_ComponentWithoutProject(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--component=my-comp"})
	cmd := newGenerateCmd()
	_ = cmd.Flags().Set("component", "my-comp")
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--component requires --project to be specified")
}

func TestGenerateCmd_ProjectWithName(t *testing.T) {
	testutil.SetOSArgs(t, []string{"occ", "componentrelease", "generate", "--project=my-proj", "--name=my-name"})
	cmd := newGenerateCmd()
	_ = cmd.Flags().Set("project", "my-proj")
	_ = cmd.Flags().Set("name", "my-name")
	err := cmd.RunE(cmd, nil)
	require.Error(t, err)
	assert.EqualError(t, err, "--name can only be used with --component (requires both --project and --component)")
}
