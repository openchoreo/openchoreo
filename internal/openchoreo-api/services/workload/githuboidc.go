// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/services"
	"github.com/openchoreo/openchoreo/internal/server/middleware/auth/githuboidc"
)

// Annotation keys propagated onto Workload CRs created via a GitHub Actions
// OIDC token. The github.com/* keys mirror the names GitHub uses in the
// token, so any reader (Backstage, CLIs, controllers) can correlate the
// Workload back to the originating workflow run without a separate lookup.
// The ci.openchoreo.dev/* key is a platform-owned discriminator that
// authorization policies and dashboards can key off without parsing the
// github.com/* keys.
const (
	AnnotationCIPlatform           = "ci.openchoreo.dev/ci-platform"
	AnnotationGitHubRepository     = "github.com/repository"
	AnnotationGitHubRef            = "github.com/ref"
	AnnotationGitHubSHA            = "github.com/sha"
	AnnotationGitHubRunID          = "github.com/run-id"
	AnnotationGitHubRunAttempt     = "github.com/run-attempt"
	AnnotationGitHubWorkflow       = "github.com/workflow"
	AnnotationGitHubWorkflowRef    = "github.com/workflow-ref"
	AnnotationGitHubJobWorkflowRef = "github.com/job-workflow-ref"

	CIPlatformGitHubActions = "github-actions"
)

// applyGitHubOIDCTrust enforces the Project-level GitHub Actions allow-list
// and stamps the resulting Workload with annotations derived from the OIDC
// token's claims. When the request was not authenticated via GitHub OIDC the
// function is a no-op and returns nil. Callers MUST invoke it before
// persisting the Workload.
func (s *workloadService) applyGitHubOIDCTrust(
	ctx context.Context,
	namespaceName string,
	w *openchoreov1alpha1.Workload,
) error {
	claims, ok := githuboidc.ClaimsFromContext(ctx)
	if !ok {
		return nil
	}

	project, err := s.lookupProject(ctx, namespaceName, w.Spec.Owner.ProjectName)
	if err != nil {
		return err
	}

	if err := authorizeGitHubClaims(project, claims); err != nil {
		s.logger.Warn("GitHub OIDC trust check failed",
			"namespace", namespaceName,
			"project", w.Spec.Owner.ProjectName,
			"repository", claims.Repository,
			"ref", claims.Ref,
			"job_workflow_ref", claims.JobWorkflowRef,
			"error", err,
		)
		return services.ErrForbidden
	}

	if w.Annotations == nil {
		w.Annotations = make(map[string]string, 9)
	}
	w.Annotations[AnnotationCIPlatform] = CIPlatformGitHubActions
	w.Annotations[AnnotationGitHubRepository] = claims.Repository
	w.Annotations[AnnotationGitHubRef] = claims.Ref
	w.Annotations[AnnotationGitHubSHA] = claims.SHA
	w.Annotations[AnnotationGitHubRunID] = claims.RunID
	w.Annotations[AnnotationGitHubRunAttempt] = claims.RunAttempt
	w.Annotations[AnnotationGitHubWorkflow] = claims.Workflow
	w.Annotations[AnnotationGitHubWorkflowRef] = claims.WorkflowRef
	w.Annotations[AnnotationGitHubJobWorkflowRef] = claims.JobWorkflowRef

	s.logger.Debug("GitHub OIDC trust check passed",
		"namespace", namespaceName,
		"project", w.Spec.Owner.ProjectName,
		"repository", claims.Repository,
	)
	return nil
}

// lookupProject fetches the Project CR that the Workload claims to belong to.
// A Workload that points at a non-existent Project under an OIDC-authenticated
// request is treated as forbidden rather than not-found because the allow-list
// is mandatory for OIDC callers.
func (s *workloadService) lookupProject(
	ctx context.Context,
	namespaceName, projectName string,
) (*openchoreov1alpha1.Project, error) {
	project := &openchoreov1alpha1.Project{}
	err := s.k8sClient.Get(ctx, client.ObjectKey{Name: projectName, Namespace: namespaceName}, project)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, services.ErrForbidden
		}
		return nil, fmt.Errorf("lookup project %s/%s: %w", namespaceName, projectName, err)
	}
	return project, nil
}

// authorizeGitHubClaims validates the OIDC token's repository, ref, and
// job_workflow_ref claims against the allow-lists declared on the Project.
// Empty allow-lists for ref and job_workflow_ref mean "no restriction" for
// that dimension; the repository allow-list however is mandatory - a Project
// that has not opted any repository in MUST NOT accept OIDC traffic.
func authorizeGitHubClaims(p *openchoreov1alpha1.Project, claims *githuboidc.Claims) error {
	if p.Spec.ExternalCI == nil || p.Spec.ExternalCI.GitHubActions == nil {
		return fmt.Errorf("project %q has no GitHub Actions allow-list", p.Name)
	}
	gha := p.Spec.ExternalCI.GitHubActions

	if !contains(gha.AllowedRepositories, claims.Repository) {
		return fmt.Errorf("repository %q not in project allow-list", claims.Repository)
	}
	if len(gha.AllowedRefs) > 0 && !contains(gha.AllowedRefs, claims.Ref) {
		return fmt.Errorf("ref %q not in project allow-list", claims.Ref)
	}
	if len(gha.AllowedJobWorkflowRefs) > 0 && !contains(gha.AllowedJobWorkflowRefs, claims.JobWorkflowRef) {
		return fmt.Errorf("job_workflow_ref %q not in project allow-list", claims.JobWorkflowRef)
	}
	return nil
}

func contains(set []string, v string) bool {
	for _, item := range set {
		if item == v {
			return true
		}
	}
	return false
}
