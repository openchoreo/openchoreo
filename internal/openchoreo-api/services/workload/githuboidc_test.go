// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/githuboidc"
)

func TestAuthorizeGitHubClaims(t *testing.T) {
	t.Parallel()
	baseClaims := func() *githuboidc.Claims {
		return &githuboidc.Claims{
			Repository:     "octo-org/octo-repo",
			Ref:            "refs/heads/main",
			JobWorkflowRef: "octo-org/octo-repo/.github/workflows/deploy.yml@refs/heads/main",
		}
	}
	project := func(repos, refs, workflows []string) *openchoreov1alpha1.Project {
		return &openchoreov1alpha1.Project{
			ObjectMeta: metav1.ObjectMeta{Name: "demo"},
			Spec: openchoreov1alpha1.ProjectSpec{
				ExternalCI: &openchoreov1alpha1.ProjectExternalCI{
					GitHubActions: &openchoreov1alpha1.ProjectGitHubActions{
						AllowedRepositories:    repos,
						AllowedRefs:            refs,
						AllowedJobWorkflowRefs: workflows,
					},
				},
			},
		}
	}

	tests := []struct {
		name    string
		project *openchoreov1alpha1.Project
		claims  *githuboidc.Claims
		wantErr bool
	}{
		{
			name:    "no externalCI block rejects",
			project: &openchoreov1alpha1.Project{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
			claims:  baseClaims(),
			wantErr: true,
		},
		{
			name:    "empty allow-list rejects",
			project: project(nil, nil, nil),
			claims:  baseClaims(),
			wantErr: true,
		},
		{
			name:    "repo allow-list match accepts",
			project: project([]string{"octo-org/octo-repo"}, nil, nil),
			claims:  baseClaims(),
			wantErr: false,
		},
		{
			name:    "repo allow-list miss rejects",
			project: project([]string{"other-org/other-repo"}, nil, nil),
			claims:  baseClaims(),
			wantErr: true,
		},
		{
			name:    "ref allow-list miss rejects",
			project: project([]string{"octo-org/octo-repo"}, []string{"refs/heads/release"}, nil),
			claims:  baseClaims(),
			wantErr: true,
		},
		{
			name:    "ref allow-list match accepts",
			project: project([]string{"octo-org/octo-repo"}, []string{"refs/heads/main"}, nil),
			claims:  baseClaims(),
			wantErr: false,
		},
		{
			name:    "workflow allow-list miss rejects",
			project: project([]string{"octo-org/octo-repo"}, nil, []string{"octo-org/octo-repo/.github/workflows/other.yml@refs/heads/main"}),
			claims:  baseClaims(),
			wantErr: true,
		},
		{
			name:    "workflow allow-list match accepts",
			project: project([]string{"octo-org/octo-repo"}, nil, []string{"octo-org/octo-repo/.github/workflows/deploy.yml@refs/heads/main"}),
			claims:  baseClaims(),
			wantErr: false,
		},
		{
			name:    "all three dimensions match accepts",
			project: project([]string{"octo-org/octo-repo"}, []string{"refs/heads/main"}, []string{"octo-org/octo-repo/.github/workflows/deploy.yml@refs/heads/main"}),
			claims:  baseClaims(),
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := authorizeGitHubClaims(tc.project, tc.claims)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestContains(t *testing.T) {
	t.Parallel()
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Fatal("expected 'b' to be found in slice")
	}
	if contains([]string{"a", "b", "c"}, "z") {
		t.Fatal("expected 'z' to not be found in slice")
	}
	if contains(nil, "anything") {
		t.Fatal("expected nil slice to never contain anything")
	}
}
