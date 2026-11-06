// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package componentrelease

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/occ/resources/client/mocks"
	"github.com/openchoreo/openchoreo/internal/occ/testutil"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// diffRelease builds a component release whose workload image and env vars are
// the only things the tests vary.
func diffRelease(name, image string, env ...string) *gen.ComponentRelease {
	envVars := make([]any, 0, len(env))
	for _, e := range env {
		envVars = append(envVars, map[string]any{"key": e, "value": "1"})
	}
	r := &gen.ComponentRelease{
		Metadata: gen.ObjectMeta{Name: name},
		Spec: &gen.ComponentReleaseSpec{
			ComponentType: map[string]any{"kind": "ComponentType", "name": "deployment/service"},
			Workload: map[string]any{
				"container": map[string]any{"image": image, "env": envVars},
			},
		},
	}
	r.Spec.Owner.ProjectName = "online-store"
	r.Spec.Owner.ComponentName = "checkout"
	return r
}

// --- Diff ---

func TestDiff_ValidationError(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).Diff(DiffParams{From: "a", To: "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--namespace")
}

func TestDiff_EmptyName(t *testing.T) {
	err := New(mocks.NewMockInterface(t)).Diff(DiffParams{Namespace: "my-org", From: "a", To: ""})
	assert.EqualError(t, err, "both component release names must be non-empty")
}

func TestDiff_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1", "A", "B"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v2", "A"), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2"}))
	})

	assert.Equal(t, `Comparing componentrelease spec: rel-1 -> rel-2

- spec.workload.container.env[1]: {"key":"B","value":"1"}
~ spec.workload.container.image: "nginx:v1" -> "nginx:v2"

2 changes: 0 added, 1 removed, 1 modified
`, out)
}

func TestDiff_Added(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v1", "A"), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2"}))
	})

	assert.Contains(t, out, `+ spec.workload.container.env[0]: {"key":"A","value":"1"}`)
	assert.Contains(t, out, "1 change: 1 added, 0 removed, 0 modified")
}

func TestDiff_IdenticalIgnoresMetadata(t *testing.T) {
	from := diffRelease("rel-1", "nginx:v1", "A")
	to := diffRelease("rel-2", "nginx:v1", "A")
	t1, t2 := time.Now().Add(-time.Hour), time.Now()
	uid1, uid2 := "uid-1", "uid-2"
	from.Metadata.CreationTimestamp, to.Metadata.CreationTimestamp = &t1, &t2
	from.Metadata.Uid, to.Metadata.Uid = &uid1, &uid2

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(from, nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(to, nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2"}))
	})

	assert.Equal(t, "No differences in spec between componentrelease \"rel-1\" and \"rel-2\"\n", out)
}

func TestDiff_NilSpec(t *testing.T) {
	from := diffRelease("rel-1", "nginx:v1")
	from.Spec = nil

	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(from, nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v1"), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2"}))
	})

	assert.Contains(t, out, "+ spec: ")
}

func TestDiff_Color(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v2"), nil)

	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2", Color: true}))
	})

	assert.Contains(t, out, "\033[33m~ spec.workload.container.image: \"nginx:v1\" -> \"nginx:v2\"\033[0m")
}

func TestDiff_FromNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("component release not found"))

	err := New(mc).Diff(DiffParams{Namespace: "my-org", From: "missing", To: "rel-2"})
	assert.EqualError(t, err, `failed to get component release "missing": component release not found`)
}

func TestDiff_ToNotFound(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "missing").Return(nil, fmt.Errorf("component release not found"))

	out := testutil.CaptureStdout(t, func() {
		err := New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "missing"})
		assert.EqualError(t, err, `failed to get component release "missing": component release not found`)
	})
	assert.Empty(t, out)
}

func TestDiff_APIError(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(nil, fmt.Errorf("unexpected response status: 500"))

	err := New(mc).Diff(DiffParams{Namespace: "my-org", From: "rel-1", To: "rel-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected response status: 500")
}

// --- diff command ---

func TestDiffCmd_Flags(t *testing.T) {
	cmd := newDiffCmd(errFactory("unused"))
	ns := cmd.Flags().Lookup("namespace")
	require.NotNil(t, ns)
	assert.Equal(t, "n", ns.Shorthand)
	color := cmd.Flags().Lookup("color")
	require.NotNil(t, color)
	assert.Equal(t, "auto", color.DefValue)
}

func TestDiffCmd_Args(t *testing.T) {
	cmd := newDiffCmd(errFactory("unused"))

	err := cmd.Args(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required argument")

	err = cmd.Args(cmd, []string{"rel-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TO_")

	err = cmd.Args(cmd, []string{"a", "b", "c"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "accepts 2 arg(s)")

	assert.NoError(t, cmd.Args(cmd, []string{"a", "b"}))
}

func TestDiffCmd_FactoryError(t *testing.T) {
	cmd := newDiffCmd(errFactory("factory failed"))
	assert.EqualError(t, cmd.RunE(cmd, []string{"a", "b"}), "factory failed")
}

func TestDiffCmd_InvalidColor(t *testing.T) {
	cmd := newDiffCmd(errFactory("factory must not be called"))
	require.NoError(t, cmd.Flags().Set("color", "rainbow"))
	err := cmd.RunE(cmd, []string{"a", "b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid --color value "rainbow"`)
}

func TestDiffCmd_Success(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v2"), nil)

	cmd := newDiffCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	require.NoError(t, cmd.Flags().Set("color", "always"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"rel-1", "rel-2"}))
	})

	assert.Contains(t, out, "\033[33m~ spec.workload.container.image")
}

func TestDiffCmd_ColorNever(t *testing.T) {
	mc := mocks.NewMockInterface(t)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-1").Return(diffRelease("rel-1", "nginx:v1"), nil)
	mc.EXPECT().GetComponentRelease(mock.Anything, "my-org", "rel-2").Return(diffRelease("rel-2", "nginx:v2"), nil)

	cmd := newDiffCmd(mockFactory(mc))
	require.NoError(t, cmd.Flags().Set("namespace", "my-org"))
	require.NoError(t, cmd.Flags().Set("color", "never"))
	out := testutil.CaptureStdout(t, func() {
		require.NoError(t, cmd.RunE(cmd, []string{"rel-1", "rel-2"}))
	})

	assert.Contains(t, out, `~ spec.workload.container.image: "nginx:v1" -> "nginx:v2"`)
	assert.NotContains(t, out, "\033[")
}

func TestDiffCmd_MissingNamespace(t *testing.T) {
	cmd := newDiffCmd(mockFactory(mocks.NewMockInterface(t)))
	err := cmd.RunE(cmd, []string{"rel-1", "rel-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "occ componentrelease diff -h")
}
