// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBranchHeadSHA = "9be21eb1421e23cd1cb02d877f8d62a06fab7a76"

func TestDetectProviderTypeFromURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		want    ProviderType
		wantErr bool
	}{
		{name: "github https", repoURL: "https://github.com/owner/repo", want: ProviderGitHub},
		{name: "github https with .git suffix", repoURL: "https://github.com/owner/repo.git", want: ProviderGitHub},
		{name: "github ssh", repoURL: "git@github.com:owner/repo.git", want: ProviderGitHub},
		{name: "github www prefix", repoURL: "https://www.github.com/owner/repo", want: ProviderGitHub},
		{name: "gitlab https", repoURL: "https://gitlab.com/group/repo", want: ProviderGitLab},
		{name: "gitlab nested group", repoURL: "https://gitlab.com/group/subgroup/repo", want: ProviderGitLab},
		{name: "bitbucket https", repoURL: "https://bitbucket.org/workspace/repo", want: ProviderBitbucket},
		{name: "unknown host", repoURL: "https://git.example.com/owner/repo", wantErr: true},
		{name: "no host", repoURL: "not-a-url", wantErr: true},
		{name: "empty", repoURL: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DetectProviderTypeFromURL(tt.repoURL)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGitHubGetBranchHead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/repos/owner/repo/branches/main", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main","commit":{"sha":"` + testBranchHeadSHA + `"}}`))
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		sha, err := p.GetBranchHead(context.Background(), "https://github.com/owner/repo.git", "main")
		require.NoError(t, err)
		assert.Equal(t, testBranchHeadSHA, sha)
	})

	t.Run("escapes branch name with slash", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/repos/owner/repo/branches/feature%2Ffoo", r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"commit":{"sha":"` + testBranchHeadSHA + `"}}`))
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		sha, err := p.GetBranchHead(context.Background(), "https://github.com/owner/repo", "feature/foo")
		require.NoError(t, err)
		assert.Equal(t, testBranchHeadSHA, sha)
	})

	t.Run("ssh repository URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/repos/owner/repo/branches/main", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"commit":{"sha":"` + testBranchHeadSHA + `"}}`))
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		sha, err := p.GetBranchHead(context.Background(), "git@github.com:owner/repo.git", "main")
		require.NoError(t, err)
		assert.Equal(t, testBranchHeadSHA, sha)
	})

	t.Run("branch not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"Branch not found"}`, http.StatusNotFound)
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://github.com/owner/repo", "missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("response without commit SHA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main"}`))
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no commit SHA")
	})

	t.Run("invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer server.Close()

		p := NewGitHubProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://github.com/owner/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode")
	})

	t.Run("repository URL without owner and repo", func(t *testing.T) {
		p := NewGitHubProvider()
		_, err := p.GetBranchHead(context.Background(), "https://github.com/owner", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "owner and repository name")
	})
}

func TestGitLabGetBranchHead(t *testing.T) {
	t.Run("success with nested group path", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/api/v4/projects/group%2Fsubgroup%2Frepo/repository/branches/main", r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main","commit":{"id":"` + testBranchHeadSHA + `"}}`))
		}))
		defer server.Close()

		p := NewGitLabProvider()
		p.apiBaseURL = server.URL

		sha, err := p.GetBranchHead(context.Background(), "https://gitlab.com/group/subgroup/repo.git", "main")
		require.NoError(t, err)
		assert.Equal(t, testBranchHeadSHA, sha)
	})

	t.Run("branch not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"404 Branch Not Found"}`, http.StatusNotFound)
		}))
		defer server.Close()

		p := NewGitLabProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://gitlab.com/group/repo", "missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("response without commit SHA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main"}`))
		}))
		defer server.Close()

		p := NewGitLabProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://gitlab.com/group/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no commit SHA")
	})

	t.Run("repository URL without project path", func(t *testing.T) {
		p := NewGitLabProvider()
		_, err := p.GetBranchHead(context.Background(), "https://gitlab.com/group", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "project path")
	})
}

func TestBitbucketGetBranchHead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/2.0/repositories/workspace/repo/refs/branches/main", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main","target":{"hash":"` + testBranchHeadSHA + `"}}`))
		}))
		defer server.Close()

		p := NewBitbucketProvider()
		p.apiBaseURL = server.URL

		sha, err := p.GetBranchHead(context.Background(), "https://bitbucket.org/workspace/repo.git", "main")
		require.NoError(t, err)
		assert.Equal(t, testBranchHeadSHA, sha)
	})

	t.Run("branch not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"type":"error"}`, http.StatusNotFound)
		}))
		defer server.Close()

		p := NewBitbucketProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://bitbucket.org/workspace/repo", "missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
	})

	t.Run("response without commit SHA", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"main"}`))
		}))
		defer server.Close()

		p := NewBitbucketProvider()
		p.apiBaseURL = server.URL

		_, err := p.GetBranchHead(context.Background(), "https://bitbucket.org/workspace/repo", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no commit SHA")
	})

	t.Run("repository URL without workspace and repo", func(t *testing.T) {
		p := NewBitbucketProvider()
		_, err := p.GetBranchHead(context.Background(), "https://bitbucket.org/workspace", "main")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "workspace and repository name")
	})
}

func TestSanitizeRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		want    string
	}{
		{
			name:    "https without credentials unchanged",
			repoURL: "https://github.com/owner/repo.git",
			want:    "https://github.com/owner/repo.git",
		},
		{
			name:    "strips userinfo token",
			repoURL: "https://user:secret-token@github.com/owner/repo.git",
			want:    "https://github.com/owner/repo.git",
		},
		{
			name:    "strips bare username",
			repoURL: "https://token@gitlab.com/group/repo",
			want:    "https://gitlab.com/group/repo",
		},
		{
			name:    "ssh form strips user",
			repoURL: "git@github.com:owner/repo.git",
			want:    "github.com:owner/repo.git",
		},
		{
			name:    "plain string unchanged",
			repoURL: "not-a-url",
			want:    "not-a-url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeRepoURL(tt.repoURL)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "secret-token")
		})
	}
}

func TestResolveBranchHeadUnknownProvider(t *testing.T) {
	_, err := ResolveBranchHead(context.Background(), "https://git.example.com/owner/repo", "main")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine git provider")
}
