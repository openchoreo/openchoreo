// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import "strconv"

// InvokeFromPodByLabel resolves a Running pod by label selector and runs
// `wget -q -O /dev/null -T <seconds> <url>` from the given container.
// Returns the combined wget output (empty on success) and the exec error.
// BusyBox-compatible (uses `-T` not `--timeout`). Treats non-2xx HTTP
// responses as failures, so this is suitable when callers expect 2xx.
// Use CheckTCPReachableFromPodByLabel for plain TCP reachability checks.
func InvokeFromPodByLabel(kubeContext, namespace, labelSelector, container, url string, timeoutSeconds int) (string, error) {
	return KubectlExecByLabel(kubeContext, namespace, labelSelector, container,
		"wget", "-q", "-O", "/dev/null", "-T", strconv.Itoa(timeoutSeconds), url)
}

// CheckTCPReachableFromPodByLabel resolves a Running pod by label selector and
// runs `nc -z -w <seconds> <host> <port>` from the given container. Returns
// the combined output and the exec error. nc exits 0 if the TCP port is
// reachable, non-zero otherwise. Use this when the workload may respond with
// any HTTP status (e.g. 404 on /) and you only care about TCP reachability.
func CheckTCPReachableFromPodByLabel(kubeContext, namespace, labelSelector, container, host, port string, timeoutSeconds int) (string, error) {
	return KubectlExecByLabel(kubeContext, namespace, labelSelector, container,
		"nc", "-z", "-w", strconv.Itoa(timeoutSeconds), host, port)
}
