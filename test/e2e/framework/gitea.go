// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Gitea credentials used by InstallGitea and the API helpers below. The values
// are intentionally fixed and only meaningful inside the e2e cluster — no
// secrets cross the cluster boundary.
const (
	GiteaAdminUser     = "e2eadmin"
	GiteaAdminPassword = "e2eAdmin12345!"
	GiteaAdminEmail    = "e2eadmin@e2e.local"
	GiteaImage         = "gitea/gitea:1.22.6"
	giteaAppName       = "gitea"
	giteaContainerPort = 3000
)

// InstallGitea applies a minimal single-replica Gitea Deployment + Service into
// the given namespace, waits for it to come up, and creates the admin user used
// by the rest of the helper API. Idempotent — re-running is a no-op once the
// deployment is Ready and the admin user exists.
func InstallGitea(kubeContext, namespace string) error {
	if _, err := Kubectl(kubeContext, "create", "namespace", namespace,
		"--dry-run=client", "-o", "yaml"); err != nil {
		return fmt.Errorf("failed to render namespace %q manifest: %w", namespace, err)
	}
	if _, err := KubectlApplyLiteral(kubeContext, giteaNamespaceYAML(namespace)); err != nil {
		return fmt.Errorf("failed to apply gitea namespace %q: %w", namespace, err)
	}
	if _, err := KubectlApplyLiteral(kubeContext, giteaWorkloadYAML(namespace)); err != nil {
		return fmt.Errorf("failed to apply gitea workload: %w", err)
	}
	if _, err := Kubectl(kubeContext,
		"-n", namespace,
		"rollout", "status", "deployment/"+giteaAppName, "--timeout=5m",
	); err != nil {
		return fmt.Errorf("gitea deployment did not become ready: %w", err)
	}
	adminListOutput, adminListErr := Kubectl(kubeContext,
		"-n", namespace,
		"exec", "deployment/"+giteaAppName, "--",
		"su", "git", "-c", "gitea admin user list --admin",
	)
	if adminListErr != nil {
		return fmt.Errorf("failed to list gitea admin users: %w (output: %s)", adminListErr, adminListOutput)
	}
	if !giteaUserExists(adminListOutput, GiteaAdminUser) {
		adminCreateOutput, adminCreateErr := Kubectl(kubeContext,
			"-n", namespace,
			"exec", "deployment/"+giteaAppName, "--",
			"su", "git", "-c", fmt.Sprintf(
				"gitea admin user create --username %s --password %s --email %s --admin --must-change-password=false",
				GiteaAdminUser, GiteaAdminPassword, GiteaAdminEmail,
			),
		)
		if adminCreateErr != nil && !strings.Contains(adminCreateOutput, "already exists") {
			return fmt.Errorf("failed to create missing gitea admin user %q: %w (output: %s; admin users: %s)",
				GiteaAdminUser, adminCreateErr, adminCreateOutput, adminListOutput)
		}
	}
	probeOutput, probeErr := giteaCurl(kubeContext, namespace, "GET", "/api/v1/user", "")
	if probeErr != nil {
		return fmt.Errorf("gitea admin user %q exists, but auth probe failed: %w (body: %s)",
			GiteaAdminUser, probeErr, probeOutput)
	}
	return nil
}

// The function doesn't check the `username` field,
// It just checks whether the response contains the given username for simplicity.
// Since this is used with explicit usernames like "e2eadmin", the function is safe to use.
// Notable that the function may produce false positives for usernames matching other column values.
func giteaUserExists(userListOutput, username string) bool {
	for _, field := range strings.Fields(userListOutput) {
		if field == username {
			return true
		}
	}
	return false
}

// GiteaInClusterURL returns the http:// URL the builder pods use to reach the
// Gitea HTTP service from inside the cluster.
func GiteaInClusterURL(namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", giteaAppName, namespace, giteaContainerPort)
}

// GiteaRepoCloneURL returns the clone URL for a repo owned by the e2e admin
// user in the in-cluster Gitea. Suitable for ClusterWorkflow `repository.url`.
func GiteaRepoCloneURL(namespace, repoName string) string {
	return fmt.Sprintf("%s/%s/%s.git", GiteaInClusterURL(namespace), GiteaAdminUser, repoName)
}

// MigrateRepo asks Gitea to clone an upstream HTTPS git URL into the admin
// user's namespace. Suitable for mirroring openchoreo/sample-workloads into
// the in-cluster Gitea so the build matrix is self-contained.
func MigrateRepo(kubeContext, giteaNamespace, repoName, cloneAddr string) error {
	body, err := json.Marshal(map[string]any{
		"clone_addr": cloneAddr,
		"repo_name":  repoName,
		"private":    false,
		"mirror":     false,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal migrate body: %w", err)
	}
	resp, err := giteaCurl(kubeContext, giteaNamespace,
		"POST", "/api/v1/repos/migrate", string(body))
	if err != nil {
		// Repo may already exist (e.g. when the suite re-runs against a
		// gitea namespace that survived a previous AfterAll's
		// --wait=false delete). Verify with GET — match EnsureGiteaRepo.
		if _, getErr := giteaCurl(kubeContext, giteaNamespace, "GET",
			fmt.Sprintf("/api/v1/repos/%s/%s", GiteaAdminUser, repoName), ""); getErr != nil {
			return fmt.Errorf("gitea migrate %q failed: %w (body: %s)", repoName, err, resp)
		}
	}
	return nil
}

// EnsureGiteaRepo creates an empty repo under the admin user if it does not
// exist. It accepts a 409 conflict response (repo already exists) as success.
func EnsureGiteaRepo(kubeContext, giteaNamespace, repoName string) error {
	body, err := json.Marshal(map[string]any{
		"name":           repoName,
		"auto_init":      true,
		"default_branch": "main",
		"private":        false,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal repo body: %w", err)
	}
	if _, err := giteaCurl(kubeContext, giteaNamespace,
		"POST", "/api/v1/user/repos", string(body)); err != nil {
		// Repo may already exist — verify by GET.
		if _, getErr := giteaCurl(kubeContext, giteaNamespace, "GET",
			fmt.Sprintf("/api/v1/repos/%s/%s", GiteaAdminUser, repoName), ""); getErr != nil {
			return fmt.Errorf("failed to ensure repo %q: create=%v, get=%v", repoName, err, getErr)
		}
	}
	return nil
}

// PushFile upserts a single file in a Gitea repo on the given branch.
// content is the raw file body (no base64 needed — the helper handles that).
// When `commitMessage` is empty a default is used.
func PushFile(kubeContext, giteaNamespace, repoName, branch, relPath string, content []byte, commitMessage string) error {
	if commitMessage == "" {
		commitMessage = fmt.Sprintf("e2e: write %s", relPath)
	}
	encoded := base64.StdEncoding.EncodeToString(content)
	apiPath := fmt.Sprintf("/api/v1/repos/%s/%s/contents/%s",
		GiteaAdminUser, repoName, strings.TrimPrefix(relPath, "/"))
	// First try PUT (update). If the file does not exist, Gitea returns 422 —
	// fall back to POST (create) on that path.
	body, err := json.Marshal(map[string]any{
		"message": commitMessage,
		"content": encoded,
		"branch":  branch,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal file body: %w", err)
	}
	resp, postErr := giteaCurl(kubeContext, giteaNamespace, "POST", apiPath, string(body))
	if postErr == nil {
		return nil
	}
	// Try PUT for updates: PUT requires an `sha` field for the previous blob.
	prev, getErr := giteaCurl(kubeContext, giteaNamespace, "GET",
		fmt.Sprintf("%s?ref=%s", apiPath, branch), "")
	if getErr != nil {
		return fmt.Errorf("push %s POST failed (%v) and GET for sha failed: %v\nPOST resp: %s",
			relPath, postErr, getErr, resp)
	}
	var meta struct {
		SHA string `json:"sha"`
	}
	if err := json.Unmarshal([]byte(prev), &meta); err != nil || meta.SHA == "" {
		return fmt.Errorf("could not decode existing file sha for %s: %w", relPath, err)
	}
	putBody, err := json.Marshal(map[string]any{
		"message": commitMessage,
		"content": encoded,
		"branch":  branch,
		"sha":     meta.SHA,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal PUT body: %w", err)
	}
	if _, err := giteaCurl(kubeContext, giteaNamespace, "PUT", apiPath, string(putBody)); err != nil {
		return fmt.Errorf("push %s PUT failed: %w", relPath, err)
	}
	return nil
}

// PushTree walks sourceDir on the local filesystem and pushes every regular
// file under it into repoName at the same relative path on `branch`. Skips
// .git and any dotfiles at the top level. Files are pushed sequentially to
// keep individual API calls small and predictable; a multi-file tree of a
// handful of YAMLs takes a few seconds.
func PushTree(kubeContext, giteaNamespace, repoName, branch, sourceDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != sourceDir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return PushFile(kubeContext, giteaNamespace, repoName, branch, filepath.ToSlash(rel), content,
			fmt.Sprintf("e2e: sync %s", rel))
	})
}

// giteaCurl runs `kubectl exec deploy/gitea -- curl ...` against Gitea's
// in-pod loopback. Doing the curl from inside the Gitea pod avoids both
// port-forwarding and adding a separate helper pod with curl baked in. The
// gitea image ships curl by default.
func giteaCurl(kubeContext, namespace, method, path, body string) (string, error) {
	args := []string{
		"-n", namespace,
		"exec", "deployment/" + giteaAppName, "--",
		"curl", "-sS", "--fail-with-body",
		"-u", fmt.Sprintf("%s:%s", GiteaAdminUser, GiteaAdminPassword),
		"-H", "Content-Type: application/json",
		"-X", method,
		fmt.Sprintf("http://127.0.0.1:%d%s", giteaContainerPort, path),
	}
	if body != "" {
		args = append(args, "--data-binary", body)
	}
	// Retry briefly — `gitea admin user create` returns before the HTTP server
	// is fully ready in cold-start scenarios.
	var (
		out string
		err error
	)
	for attempt := 0; attempt < 10; attempt++ {
		out, err = Kubectl(kubeContext, args...)
		if err == nil {
			return out, nil
		}
		time.Sleep(2 * time.Second)
	}
	return out, err
}

func giteaNamespaceYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    openchoreo.dev/e2e-managed: "true"
`, namespace)
}

// giteaWorkloadYAML is intentionally a string template so the helper has zero
// dependency on the openchoreo API types — Gitea isn't an openchoreo concern,
// and keeping it as raw YAML matches the texture of `coredns-custom.yaml` and
// `secretstore.yaml` under test/e2e/k3d/.
func giteaWorkloadYAML(namespace string) string {
	return fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %[1]s
  template:
    metadata:
      labels:
        app: %[1]s
    spec:
      containers:
        - name: %[1]s
          image: %[3]s
          env:
            - name: INSTALL_LOCK
              value: "true"
            - name: RUN_MODE
              value: prod
            - name: GITEA__server__DISABLE_SSH
              value: "true"
            - name: GITEA__server__OFFLINE_MODE
              value: "true"
            - name: GITEA__service__DISABLE_REGISTRATION
              value: "true"
            - name: GITEA__database__DB_TYPE
              value: sqlite3
            - name: GITEA__security__INSTALL_LOCK
              value: "true"
          ports:
            - containerPort: %[4]d
              name: http
          readinessProbe:
            httpGet:
              path: /api/healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 5
          volumeMounts:
            - name: data
              mountPath: /data
      volumes:
        - name: data
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: %[2]s
spec:
  selector:
    app: %[1]s
  ports:
    - port: %[4]d
      targetPort: http
      name: http
`, giteaAppName, namespace, GiteaImage, giteaContainerPort)
}
