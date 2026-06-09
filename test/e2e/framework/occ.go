// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
)

type OCCRunner struct {
	BinaryPath string   // Path to compiled occ binary
	HomeDir    string   // Isolated $HOME directory for occ config files
	APIServer  string   // Base URL of the openchoreo-api (e.g., "http://localhost:12345")
	Env        []string // Additional environment variables (KEY=VALUE format)
}

// NewOCCRunner creates an isolated occ runner for e2e tests.
func NewOCCRunner(apiServer string) (*OCCRunner, error) {
	tempDir, err := os.MkdirTemp("", "openchoreo-occ-e2e-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create occ temp dir: %w", err)
	}

	homeDir := filepath.Join(tempDir, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to create occ home dir: %w", err)
	}

	return &OCCRunner{
		BinaryPath: filepath.Join(tempDir, "occ"),
		HomeDir:    homeDir,
		APIServer:  apiServer,
	}, nil
}

// Build compiles the occ CLI binary into the runner's temp directory.
func (r *OCCRunner) Build() error {
	repoRoot, err := RepoRoot()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	args := []string{"build", "-o", r.BinaryPath, "./cmd/occ/"}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = repoRoot
	fmt.Fprintf(GinkgoWriter, "running: go %s\n", strings.Join(args, " "))

	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if output != "" {
		fmt.Fprintf(GinkgoWriter, "output:\n%s\n", output)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("go build timed out after 5m")
	}
	if err != nil {
		return fmt.Errorf("go %s failed: %w\n%s", strings.Join(args, " "), err, output)
	}
	return nil
}

// Run executes occ with an isolated HOME and returns trimmed stdout and stderr.
func (r *OCCRunner) Run(args ...string) (stdout string, stderr string, err error) {
	return r.RunWithEnv(nil, args...)
}

// RunWithEnv executes occ with additional per-call environment variables.
// Extra env vars are appended last so they can override inherited values.
func (r *OCCRunner) RunWithEnv(extraEnv []string, args ...string) (stdout string, stderr string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.BinaryPath, args...)
	// GODEBUG=netdns=go forces the pure-Go DNS resolver, bypassing macOS mDNS
	// which adds a 5s timeout per request for .local domains.
	cmd.Env = append(os.Environ(), "HOME="+r.HomeDir, "GODEBUG=netdns=go")
	cmd.Env = append(cmd.Env, r.Env...)
	cmd.Env = append(cmd.Env, extraEnv...)

	var stdoutBuffer bytes.Buffer
	var stderrBuffer bytes.Buffer
	cmd.Stdout = &stdoutBuffer
	cmd.Stderr = &stderrBuffer

	fmt.Fprintf(GinkgoWriter, "running: %s %s\n", r.BinaryPath, strings.Join(args, " "))
	err = cmd.Run()

	stdout = strings.TrimSpace(stdoutBuffer.String())
	stderr = strings.TrimSpace(stderrBuffer.String())
	if stdout != "" {
		fmt.Fprintf(GinkgoWriter, "stdout:\n%s\n", stdout)
	}
	if stderr != "" {
		fmt.Fprintf(GinkgoWriter, "stderr:\n%s\n", stderr)
	}
	if ctx.Err() == context.DeadlineExceeded {
		err = ctx.Err()
	}
	if err != nil {
		return stdout, stderr, fmt.Errorf("occ %s failed: %w\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, stdout, stderr)
	}
	return stdout, stderr, nil
}

// SeedConfig writes an occ config file into the runner's isolated HOME.
func (r *OCCRunner) SeedConfig(token string) error {
	configDir := filepath.Join(r.HomeDir, ".openchoreo")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("failed to create occ config dir: %w", err)
	}

	config := fmt.Sprintf(`currentContext: e2e
controlplanes:
  - name: e2e
    url: %s
credentials:
  - name: e2e-creds
    clientId: customer-portal-client
    clientSecret: supersecret
    token: %s
    authMethod: client_credentials
contexts:
  - name: e2e
    controlplane: e2e
    credentials: e2e-creds
`, r.APIServer, token)

	configPath := filepath.Join(configDir, "config")
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		return fmt.Errorf("failed to write occ config: %w", err)
	}
	return nil
}

// WriteFixtureFile writes YAML content to a unique fixture file for occ apply.
func (r *OCCRunner) WriteFixtureFile(content string) (string, error) {
	fixturesDir := filepath.Join(r.HomeDir, "fixtures")
	if err := os.MkdirAll(fixturesDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create occ fixtures dir: %w", err)
	}

	fixturePath := filepath.Join(fixturesDir, fmt.Sprintf("fixture-%d.yaml", time.Now().UnixNano()))
	if err := os.WriteFile(fixturePath, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("failed to write occ fixture file: %w", err)
	}
	return fixturePath, nil
}

// Cleanup removes the runner's temp directory containing the binary and HOME.
func (r *OCCRunner) Cleanup() {
	if r == nil || r.BinaryPath == "" {
		return
	}
	_ = os.RemoveAll(filepath.Dir(r.BinaryPath))
}
