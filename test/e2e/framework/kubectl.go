// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
)

// Kubectl executes an arbitrary kubectl command with the given context.
// Returns trimmed combined output (stdout+stderr) and any error.
func Kubectl(kubeContext string, args ...string) (string, error) {
	cmdArgs := append([]string{"--context", kubeContext}, args...)
	cmd := exec.Command("kubectl", cmdArgs...)
	fmt.Fprintf(GinkgoWriter, "running: kubectl %s\n", strings.Join(cmdArgs, " "))
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))
	if err != nil {
		return output, fmt.Errorf("kubectl %s failed: %w\n%s", strings.Join(args, " "), err, output)
	}
	return output, nil
}

// KubectlGet runs: kubectl get <resource> -n <namespace> [extraArgs...]
func KubectlGet(kubeContext, namespace, resource string, extraArgs ...string) (string, error) {
	args := []string{"get", resource, "-n", namespace}
	args = append(args, extraArgs...)
	return Kubectl(kubeContext, args...)
}

// KubectlGetJsonpath runs: kubectl get <resource> <name> -n <namespace> -o jsonpath=<expr>
func KubectlGetJsonpath(kubeContext, namespace, resource, name, jsonpath string) (string, error) {
	return Kubectl(kubeContext, "get", resource, name, "-n", namespace,
		"-o", fmt.Sprintf("jsonpath=%s", jsonpath))
}

// KubectlLogs runs: kubectl logs -n <namespace> -l <labelSelector> --tail=<tail>
func KubectlLogs(kubeContext, namespace, labelSelector string, tail int) (string, error) {
	return Kubectl(kubeContext, "logs", "-n", namespace, "-l", labelSelector,
		"--tail", fmt.Sprintf("%d", tail))
}
