// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package bindingrevision

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

func TestCheckMessage(t *testing.T) {
	assert.NoError(t, CheckMessage(""))
	assert.NoError(t, CheckMessage(strings.Repeat("é", 1024)), "counted in characters, not bytes")
	assert.EqualError(t, CheckMessage(strings.Repeat("x", 1025)), "--message must be at most 1024 characters (got 1025)")
}

func TestWithChangeMessage(t *testing.T) {
	meta := gen.ObjectMeta{}
	WithChangeMessage(&meta, "")
	assert.Nil(t, meta.Annotations, "no message adds nothing")

	existing := map[string]string{openchoreov1alpha1.AnnotationKeyRevisionCause: "Promote"}
	meta.Annotations = &existing
	WithChangeMessage(&meta, "Fix checkout timeout")
	assert.Equal(t, map[string]string{
		openchoreov1alpha1.AnnotationKeyRevisionCause: "Promote",
		openchoreov1alpha1.AnnotationKeyChangeMessage: "Fix checkout timeout",
	}, *meta.Annotations, "other annotations are kept")
}

func TestRollbackRequest_SendsTheMessageOnlyWhenApplied(t *testing.T) {
	rev := int64(2)
	preview := RollbackRequest(&rev, true, "Payments failing")
	assert.Nil(t, preview.Message, "a preview changes nothing to describe")
	applied := RollbackRequest(&rev, false, "Payments failing")
	require.NotNil(t, applied.Message)
	assert.Equal(t, "Payments failing", *applied.Message)
	assert.Nil(t, RollbackRequest(&rev, false, "").Message)
}

func TestPrintTable_ShowsAShortenedMessage(t *testing.T) {
	var buf bytes.Buffer
	long := "Roll forward after the payments\nfix " + strings.Repeat("and more ", 10)
	require.NoError(t, PrintTable(&buf, []Row{{Name: "api-aaa-r1", Cause: "Deploy", Message: long}}))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2, "a multi-line message stays on one table row")
	assert.True(t, strings.HasSuffix(lines[0], "MESSAGE"))
	assert.Contains(t, lines[1], "Roll forward after the payments fix and more")
	assert.True(t, strings.HasSuffix(lines[1], "…"), "cut to the table width: %q", lines[1])
	assert.Equal(t, tableMessageLength, len([]rune(shortMessage(long))))
}
