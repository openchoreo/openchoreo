// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitcommitrequest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	choreov1 "github.com/openchoreo/openchoreo/api/v1"
)

// Reconciler reconciles a GitCommitRequest object
type Reconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.choreo.dev,resources=gitcommitrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.choreo.dev,resources=gitcommitrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.choreo.dev,resources=gitcommitrequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GitCommitRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.1/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Workload instance for this reconcile request
	gcr := &choreov1.GitCommitRequest{}
	if err := r.Get(ctx, req.NamespacedName, gcr); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "Failed to get Workload")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Skip if already done
	if gcr.Status.Phase == "Succeeded" {
		return ctrl.Result{}, nil
	}

	// 1. Build Git auth
	var auth transport.AuthMethod
	if gcr.Spec.AuthSecretRef != "" {
		sec := &corev1.Secret{}
		if err := r.Get(ctx,
			types.NamespacedName{Name: gcr.Spec.AuthSecretRef, Namespace: gcr.Namespace}, sec); err != nil {
			return r.fail(ctx, gcr, fmt.Errorf("secret: %w", err))
		}
		if user, ok := sec.Data["username"]; ok {
			auth = &http.BasicAuth{
				Username: string(user),
				Password: string(sec.Data["password"]),
			}
		}
		// else if key, ok := sec.Data["ssh-privatekey"]; ok {
		// signer, _ := ssh.ParsePrivateKey(key)                  // import "golang.org/x/crypto/ssh"
		// auth = &gitssh.PublicKeys{User: "git", Signer: signer} // import gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
		// }
	}

	// 2. Clone repo to a tmp dir
	tmp, err := os.MkdirTemp("", "repo-*")
	if err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to create temp directory: %w", err))
	}
	// Ensure cleanup of temp directory
	defer func() {
		if cleanupErr := os.RemoveAll(tmp); cleanupErr != nil {
			logger.Error(cleanupErr, "Failed to cleanup temp directory", "path", tmp)
		}
	}()

	repo, err := git.PlainCloneContext(ctx, tmp, false, &git.CloneOptions{
		URL:           gcr.Spec.RepoURL,
		ReferenceName: plumbing.NewBranchReferenceName(gcr.Spec.Branch),
		SingleBranch:  true,
		Depth:         1,
		Auth:          auth,
	})
	if err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to clone repository: %w", err))
	}

	// 3. Mutate files
	if err := applyEdits(tmp, gcr.Spec.Files); err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to apply file edits: %w", err))
	}
	wt, err := repo.Worktree()
	if err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to get worktree: %w", err))
	}
	if _, err := wt.Add("."); err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to stage changes: %w", err))
	}
	commit, err := wt.Commit(gcr.Spec.Message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  gcr.Spec.Author.Name,
			Email: gcr.Spec.Author.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return r.fail(ctx, gcr, fmt.Errorf("failed to create commit: %w", err))
	}
	// 4. Push
	if err := repo.Push(&git.PushOptions{Auth: auth}); err != nil &&
		!errors.Is(err, git.NoErrAlreadyUpToDate) {
		return r.fail(ctx, gcr, fmt.Errorf("failed to push commit: %w", err))
	}

	// 5. Update status
	gcr.Status.Phase = "Succeeded"
	gcr.Status.ObservedSHA = commit.String()
	gcr.Status.ObservedBranch = gcr.Spec.Branch
	gcr.Status.Message = "commit pushed"
	_ = r.Status().Update(ctx, gcr)

	logger.Info("Git commit completed", "sha", commit.String())
	return ctrl.Result{}, nil
}

// helper to set failed status once
//
//nolint:unparam
func (r *Reconciler) fail(ctx context.Context,
	gcr *choreov1.GitCommitRequest, err error) (ctrl.Result, error) {
	gcr.Status.Phase = "Failed"
	gcr.Status.Message = err.Error()
	_ = r.Status().Update(ctx, gcr)
	return ctrl.Result{}, err
}

func applyEdits(root string, edits []choreov1.FileEdit) error {
	for _, e := range edits {
		abs := filepath.Join(root, e.Path)
		if err := os.MkdirAll(filepath.Dir(abs), fs.ModePerm); err != nil {
			return err
		}
		if e.Patch != "" {
			original, _ := os.ReadFile(abs)
			p, _ := jsonpatch.DecodePatch([]byte(e.Patch))
			mod, err := p.Apply(original)
			if err != nil {
				return err
			}
			e.Content = string(mod)
		}
		if err := os.WriteFile(abs, []byte(e.Content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&choreov1.GitCommitRequest{}).
		Named("gitcommitrequest").
		Complete(r)
}
