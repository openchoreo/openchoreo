// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
)

// PortForward manages a kubectl port-forward background process.
type PortForward struct {
	KubeContext string
	Namespace   string
	Service     string
	LocalPort   int
	RemotePort  int
	cmd         *exec.Cmd
}

// Start begins port-forwarding in the background and waits until the port is
// accepting connections (or 15 s elapse).
func (pf *PortForward) Start() error {
	args := []string{
		"--context", pf.KubeContext,
		"port-forward",
		"-n", pf.Namespace,
		fmt.Sprintf("svc/%s", pf.Service),
		fmt.Sprintf("%d:%d", pf.LocalPort, pf.RemotePort),
	}

	pf.cmd = exec.Command("kubectl", args...)
	fmt.Fprintf(GinkgoWriter, "starting: kubectl %s\n", strings.Join(args, " "))

	stderr, err := pf.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("port-forward stderr pipe: %w", err)
	}

	if err := pf.cmd.Start(); err != nil {
		return fmt.Errorf("port-forward start: %w", err)
	}

	// Drain stderr in background so the process doesn't block.
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintf(GinkgoWriter, "port-forward[%s/%s]: %s\n", pf.Namespace, pf.Service, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(GinkgoWriter, "port-forward[%s/%s] stderr read error: %v\n", pf.Namespace, pf.Service, err)
		}
	}()

	// Poll until the local port accepts TCP connections.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", pf.LocalAddress(), 500*time.Millisecond)
		if dialErr == nil {
			conn.Close()
			fmt.Fprintf(GinkgoWriter, "port-forward ready: %s → %s/%s:%d\n",
				pf.LocalAddress(), pf.Namespace, pf.Service, pf.RemotePort)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	pf.Stop()
	return fmt.Errorf("port-forward to %s/%s:%d did not become ready within 15s", pf.Namespace, pf.Service, pf.RemotePort)
}

// Stop terminates the port-forward process.
func (pf *PortForward) Stop() {
	if pf.cmd != nil && pf.cmd.Process != nil {
		_ = pf.cmd.Process.Kill()
		_ = pf.cmd.Wait()
		fmt.Fprintf(GinkgoWriter, "port-forward stopped: %s\n", pf.LocalAddress())
	}
}

// LocalAddress returns "localhost:<LocalPort>".
func (pf *PortForward) LocalAddress() string {
	return fmt.Sprintf("localhost:%d", pf.LocalPort)
}
