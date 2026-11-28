// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	"github.com/openchoreo/openchoreo/internal/openchoreo-api/api/gen"
)

// A revision delete is never sent on after a redirect: one that drops the dryRun
// query would delete the revision a preview only checks, and one that turns the
// DELETE into a GET would read as a successful delete.
func TestRevisionDelete_DoesNotFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusMovedPermanently} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			reached := 0
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached++
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"revisionName":"api-dev-r2","revision":2,"binding":"api-dev","environment":"dev","dryRun":false}`))
			}))
			t.Cleanup(target.Close)
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// The redirect drops the query, dryRun=true included.
				http.Redirect(w, r, target.URL+r.URL.Path, status)
			}))
			t.Cleanup(proxy.Close)

			genClient, err := gen.NewClientWithResponses(proxy.URL, gen.WithHTTPClient(newHTTPClient()))
			require.NoError(t, err)
			c := &Client{client: genClient}

			_, err = c.DeleteReleaseBindingRevision(context.Background(), "default", "api-dev-r2",
				&gen.DeleteReleaseBindingRevisionParams{Binding: ptr.To("api-dev"), DryRun: ptr.To(true)})
			require.ErrorContains(t, err, "refusing to follow a redirect")
			assert.Zero(t, reached, "nothing reaches the redirect target")

			_, err = c.DeleteResourceReleaseBindingRevision(context.Background(), "default", "db-dev-r2",
				&gen.DeleteResourceReleaseBindingRevisionParams{Binding: ptr.To("db-dev"), DryRun: ptr.To(true)})
			require.ErrorContains(t, err, "refusing to follow a redirect")
			assert.Zero(t, reached)
		})
	}
}
