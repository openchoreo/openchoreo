// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const inputArtifactDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRejectUnsupportedWorkflowRunInputArtifacts(t *testing.T) {
	valid := `{"spec":{"inputArtifacts":[{"name":"unified-diff","mediaType":"text/x-diff","sizeBytes":1,"sha256":"` + inputArtifactDigest + `","expiresAt":"2030-01-01T00:00:00Z","gcs":{"bucket":"workflow-inputs","key":"unified-diff/delivery-123/` + inputArtifactDigest + `.diff"}}]}}`

	for name, body := range map[string]string{
		"accepts canonical GCS artifact":  valid,
		"accepts omitted input artifacts": `{"spec":{}}`,
		"rejects S3":                      strings.Replace(valid, `"gcs":{`, `"s3":{"bucket":"forbidden"},"gcs":{`, 1),
		"rejects URL":                     strings.Replace(valid, `"gcs":{`, `"url":"https://forbidden","gcs":{`, 1),
		"rejects inline bytes":            strings.Replace(valid, `"gcs":{`, `"bytes":"Zm9yYmlkZGVu","gcs":{`, 1),
		"rejects GCS generation":          strings.Replace(valid, `"key":`, `"generation":"1","key":`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			called := false
			h := RejectUnsupportedWorkflowRunInputArtifacts(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/namespaces/default/workflowruns", strings.NewReader(body))
			recorder := httptest.NewRecorder()

			h.ServeHTTP(recorder, req)
			if strings.HasPrefix(name, "accepts") {
				require.True(t, called)
				require.Equal(t, http.StatusOK, recorder.Code)
			} else {
				require.False(t, called)
				require.Equal(t, http.StatusBadRequest, recorder.Code)
			}
		})
	}
}
