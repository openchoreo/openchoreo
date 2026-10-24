// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchComponentLogs_ReleaseLineScope(t *testing.T) {
	for name, tc := range map[string]struct {
		req       ComponentLogsRequest
		wantScope map[string]any
	}{
		"default line": {
			req:       ComponentLogsRequest{NamespaceName: "ns", ComponentName: "comp"},
			wantScope: map[string]any{"namespace": "ns", "component": "comp"},
		},
		"one line": {
			req:       ComponentLogsRequest{NamespaceName: "ns", ComponentName: "comp", ReleaseLine: "next"},
			wantScope: map[string]any{"namespace": "ns", "component": "comp", "releaseLine": "next"},
		},
		"every line": {
			req:       ComponentLogsRequest{NamespaceName: "ns", ComponentName: "comp", AllReleaseLines: true},
			wantScope: map[string]any{"namespace": "ns", "component": "comp", "allReleaseLines": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var body struct {
				SearchScope map[string]any `json:"searchScope"`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"logs":[]}`))
			}))
			defer server.Close()

			_, err := NewObserverClient(server.URL, "").FetchComponentLogs(context.Background(), tc.req)
			require.NoError(t, err)
			for key, want := range tc.wantScope {
				assert.Equal(t, want, body.SearchScope[key], key)
			}
			for _, key := range []string{"releaseLine", "allReleaseLines"} {
				if _, wanted := tc.wantScope[key]; !wanted {
					assert.NotContains(t, body.SearchScope, key)
				}
			}
		})
	}
}
