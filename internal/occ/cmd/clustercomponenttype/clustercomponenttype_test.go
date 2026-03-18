// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustercomponenttype

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// captureStdout captures stdout output from a function call.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		w.Close()
		r.Close()
	}()

	fn()

	os.Stdout = origStdout
	w.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)

	return buf.String()
}

func TestPrint_Nil(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList(nil))
	})
	assert.Contains(t, out, "No cluster component types found")
}

func TestPrint_Empty(t *testing.T) {
	out := captureStdout(t, func() {
		require.NoError(t, printList([]gen.ClusterComponentType{}))
	})
	assert.Contains(t, out, "No cluster component types found")
}

func TestPrint_WithItems(t *testing.T) {
	now := time.Now()
	workloadType := gen.ClusterComponentTypeSpecWorkloadTypeDeployment
	items := []gen.ClusterComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "web-app",
				CreationTimestamp: &now,
			},
			Spec: &gen.ClusterComponentTypeSpec{
				WorkloadType: workloadType,
			},
		},
		{
			Metadata: gen.ObjectMeta{
				Name: "batch-job",
			},
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "NAME")
	assert.Contains(t, out, "WORKLOAD TYPE")
	assert.Contains(t, out, "AGE")
	assert.Contains(t, out, "web-app")
	assert.Contains(t, out, "deployment")
	assert.Contains(t, out, "batch-job")
}

func TestPrint_NilSpec(t *testing.T) {
	now := time.Now()
	items := []gen.ClusterComponentType{
		{
			Metadata: gen.ObjectMeta{
				Name:              "no-spec-type",
				CreationTimestamp: &now,
			},
			Spec: nil,
		},
	}

	out := captureStdout(t, func() {
		require.NoError(t, printList(items))
	})

	assert.Contains(t, out, "no-spec-type")
}
