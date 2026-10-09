// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var inputArtifactFields = map[string]struct{}{
	"expiresAt": {}, "gcs": {}, "mediaType": {}, "name": {}, "sha256": {}, "sizeBytes": {},
}

var inputArtifactGCSFields = map[string]struct{}{
	"bucket": {}, "key": {},
}

// RejectUnsupportedWorkflowRunInputArtifacts rejects fields outside the immutable
// input-artifact contract before the generated JSON decoder can discard them.
// This makes attempts to send inline bytes, URLs, S3 locations, or credentials a
// client error instead of silently accepting a modified request.
func RejectUnsupportedWorkflowRunInputArtifacts(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isWorkflowRunMutation(r) {
			next.ServeHTTP(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid workflow run request", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		if err := validateInputArtifactFields(body); err != nil {
			http.Error(w, fmt.Sprintf("invalid inputArtifacts: %v", err), http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isWorkflowRunMutation(r *http.Request) bool {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		return false
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "namespaces" || parts[4] != "workflowruns" {
		return false
	}
	return (r.Method == http.MethodPost && len(parts) == 5) || (r.Method == http.MethodPut && len(parts) == 6)
}

func validateInputArtifactFields(body []byte) error {
	var request struct {
		Spec struct {
			InputArtifacts json.RawMessage `json:"inputArtifacts"`
		} `json:"spec"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		// The generated handler reports malformed request bodies consistently.
		return nil
	}
	if len(request.Spec.InputArtifacts) == 0 || string(request.Spec.InputArtifacts) == "null" {
		return nil
	}

	var artifacts []json.RawMessage
	if err := json.Unmarshal(request.Spec.InputArtifacts, &artifacts); err != nil {
		return nil
	}
	for _, artifact := range artifacts {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(artifact, &fields); err != nil {
			return nil
		}
		if err := rejectUnknownFields(fields, inputArtifactFields); err != nil {
			return err
		}
		if gcs, ok := fields["gcs"]; ok {
			var gcsFields map[string]json.RawMessage
			if err := json.Unmarshal(gcs, &gcsFields); err == nil {
				if err := rejectUnknownFields(gcsFields, inputArtifactGCSFields); err != nil {
					return fmt.Errorf("gcs.%w", err)
				}
			}
		}
	}
	return nil
}

func rejectUnknownFields(fields map[string]json.RawMessage, allowed map[string]struct{}) error {
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("field %q is not allowed", field)
		}
	}
	return nil
}
