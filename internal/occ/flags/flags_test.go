// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package flags

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func newTestCmd() *cobra.Command {
	return &cobra.Command{Use: "test"}
}

// --- String flag tests ---

func TestNamespace_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddNamespace(cmd)

	assert.Equal(t, "", GetNamespace(cmd))

	_ = cmd.Flags().Set("namespace", "acme-corp")
	assert.Equal(t, "acme-corp", GetNamespace(cmd))
}

func TestNamespace_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddNamespace(cmd)

	f := cmd.Flags().Lookup("namespace")
	assert.Equal(t, "n", f.Shorthand)
}

func TestProject_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddProject(cmd)

	assert.Equal(t, "", GetProject(cmd))

	_ = cmd.Flags().Set("project", "online-store")
	assert.Equal(t, "online-store", GetProject(cmd))
}

func TestProject_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddProject(cmd)

	f := cmd.Flags().Lookup("project")
	assert.Equal(t, "p", f.Shorthand)
}

func TestComponent_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddComponent(cmd)

	assert.Equal(t, "", GetComponent(cmd))

	_ = cmd.Flags().Set("component", "product-catalog")
	assert.Equal(t, "product-catalog", GetComponent(cmd))
}

func TestComponent_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddComponent(cmd)

	f := cmd.Flags().Lookup("component")
	assert.Equal(t, "c", f.Shorthand)
}

func TestEnvironment_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddEnvironment(cmd)

	assert.Equal(t, "", GetEnvironment(cmd))

	_ = cmd.Flags().Set("env", "staging")
	assert.Equal(t, "staging", GetEnvironment(cmd))
}

func TestSince_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddSince(cmd)

	assert.Equal(t, "", GetSince(cmd))

	_ = cmd.Flags().Set("since", "5m")
	assert.Equal(t, "5m", GetSince(cmd))
}

func TestMode_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddMode(cmd)

	assert.Equal(t, "", GetMode(cmd))

	_ = cmd.Flags().Set("mode", ModeFileSystem)
	assert.Equal(t, ModeFileSystem, GetMode(cmd))
}

func TestRootDir_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddRootDir(cmd)

	assert.Equal(t, "", GetRootDir(cmd))

	_ = cmd.Flags().Set("root-dir", "/tmp/project")
	assert.Equal(t, "/tmp/project", GetRootDir(cmd))
}

func TestOutputPath_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddOutputPath(cmd)

	assert.Equal(t, "", GetOutputPath(cmd))

	_ = cmd.Flags().Set("output-path", "/tmp/out")
	assert.Equal(t, "/tmp/out", GetOutputPath(cmd))
}

func TestOutputPath_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddOutputPath(cmd)

	f := cmd.Flags().Lookup("output-path")
	assert.Equal(t, "o", f.Shorthand)
}

func TestRelease_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddRelease(cmd)

	assert.Equal(t, "", GetRelease(cmd))

	_ = cmd.Flags().Set("release", "v1.0.0")
	assert.Equal(t, "v1.0.0", GetRelease(cmd))
}

func TestTo_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddTo(cmd)

	assert.Equal(t, "", GetTo(cmd))

	_ = cmd.Flags().Set("to", "production")
	assert.Equal(t, "production", GetTo(cmd))
}

func TestTargetEnv_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddTargetEnv(cmd)

	assert.Equal(t, "", GetTargetEnv(cmd))

	_ = cmd.Flags().Set("target-env", "prod")
	assert.Equal(t, "prod", GetTargetEnv(cmd))
}

func TestTargetEnv_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddTargetEnv(cmd)

	f := cmd.Flags().Lookup("target-env")
	assert.Equal(t, "e", f.Shorthand)
}

func TestWorkflowRun_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddWorkflowRun(cmd)

	assert.Equal(t, "", GetWorkflowRun(cmd))

	_ = cmd.Flags().Set("workflowrun", "run-123")
	assert.Equal(t, "run-123", GetWorkflowRun(cmd))
}

func TestOutputFile_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddOutputFile(cmd)

	assert.Equal(t, "", GetOutputFile(cmd))

	_ = cmd.Flags().Set("output-file", "result.yaml")
	assert.Equal(t, "result.yaml", GetOutputFile(cmd))
}

func TestOutputFile_HasShorthand(t *testing.T) {
	cmd := newTestCmd()
	AddOutputFile(cmd)

	f := cmd.Flags().Lookup("output-file")
	assert.Equal(t, "o", f.Shorthand)
}

func TestUsePipeline_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddUsePipeline(cmd)

	assert.Equal(t, "", GetUsePipeline(cmd))

	_ = cmd.Flags().Set("use-pipeline", "main-pipeline")
	assert.Equal(t, "main-pipeline", GetUsePipeline(cmd))
}

func TestComponentRelease_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddComponentRelease(cmd)

	assert.Equal(t, "", GetComponentRelease(cmd))

	_ = cmd.Flags().Set("component-release", "cr-v1")
	assert.Equal(t, "cr-v1", GetComponentRelease(cmd))
}

func TestControlPlane_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddControlPlane(cmd)

	assert.Equal(t, "", GetControlPlane(cmd))

	_ = cmd.Flags().Set("controlplane", "cp-1")
	assert.Equal(t, "cp-1", GetControlPlane(cmd))
}

func TestCredentials_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddCredentials(cmd)

	assert.Equal(t, "", GetCredentials(cmd))

	_ = cmd.Flags().Set("credentials", "cred-1")
	assert.Equal(t, "cred-1", GetCredentials(cmd))
}

func TestURL_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddURL(cmd)

	assert.Equal(t, "", GetURL(cmd))

	_ = cmd.Flags().Set("url", "https://example.com")
	assert.Equal(t, "https://example.com", GetURL(cmd))
}

// --- Bool flag tests ---

func TestFollow_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddFollow(cmd)

	assert.False(t, GetFollow(cmd))

	_ = cmd.Flags().Set("follow", "true")
	assert.True(t, GetFollow(cmd))
}

func TestDryRun_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddDryRun(cmd)

	assert.False(t, GetDryRun(cmd))

	_ = cmd.Flags().Set("dry-run", "true")
	assert.True(t, GetDryRun(cmd))
}

func TestAll_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddAll(cmd)

	assert.False(t, GetAll(cmd))

	_ = cmd.Flags().Set("all", "true")
	assert.True(t, GetAll(cmd))
}

// --- Int flag tests ---

func TestTail_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddTail(cmd)

	assert.Equal(t, 0, GetTail(cmd))

	_ = cmd.Flags().Set("tail", "100")
	assert.Equal(t, 100, GetTail(cmd))
}

// --- StringArray flag tests ---

func TestSet_DefaultAndSet(t *testing.T) {
	cmd := newTestCmd()
	AddSet(cmd)

	assert.Empty(t, GetSet(cmd))

	_ = cmd.Flags().Set("set", "type.replicas=3")
	_ = cmd.Flags().Set("set", "type.image=nginx")
	assert.Equal(t, []string{"type.replicas=3", "type.image=nginx"}, GetSet(cmd))
}

// --- Mode constants ---

func TestModeConstants(t *testing.T) {
	assert.Equal(t, "api-server", ModeAPIServer)
	assert.Equal(t, "file-system", ModeFileSystem)
}
