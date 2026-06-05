// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package quickstart

import (
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
)

const (
	containerName = "openchoreo-qs-e2e-test"
	containerUser = "openchoreo"
	containerHome = "/home/openchoreo"
)

func startContainer(image string) error {
	cleanupContainer()

	// The entrypoint normally sets up docker socket permissions and starts an
	// interactive shell.  We override it to run the permission setup inline
	// and keep the container alive with "sleep infinity".
	initScript := `
if [ -S /var/run/docker.sock ]; then
  GID=$(stat -c '%g' /var/run/docker.sock 2>/dev/null || stat -f '%g' /var/run/docker.sock 2>/dev/null || echo 0)
  if [ "$GID" = "0" ]; then
    addgroup openchoreo root 2>/dev/null || true
    chmod g+rw /var/run/docker.sock 2>/dev/null || true
  else
    getent group "$GID" >/dev/null 2>&1 || addgroup -g "$GID" docker 2>/dev/null || true
    GN=$(getent group "$GID" | cut -d: -f1)
    addgroup openchoreo "${GN:-docker}" 2>/dev/null || true
  fi
fi
exec sleep infinity`

	cmd := exec.Command("docker", "run", "-d",
		"--name", containerName,
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"--network=host",
		"--entrypoint", "",
		image,
		"sh", "-c", initScript,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w\n%s", err, string(out))
	}
	return nil
}

func cleanupContainer() {
	exec.Command("docker", "rm", "-f", containerName).Run() //nolint:errcheck
}

// dockerExec runs a command inside the container as the openchoreo user.
func dockerExec(script string) (string, error) {
	cmd := exec.Command("docker", "exec",
		"--user", containerUser,
		"--workdir", containerHome,
		containerName,
		"bash", "-lc", script,
	)
	fmt.Fprintf(GinkgoWriter, "docker exec: %s\n", truncate(script, 120))
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("docker exec failed: %w\n%s", err, output)
	}
	return output, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
