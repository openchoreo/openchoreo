// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package framework

import (
	"fmt"
	"strconv"
)

// GenerateHTTPTraffic runs a fire-and-forget request loop from a Running pod
// to a target URL. It blocks for roughly `durationSec * rps` requests (modulo
// sleep granularity) so the caller knows when traffic generation has settled.
//
// Implementation: a single `kubectl exec` runs a tiny `sh -c` loop in the
// tester pod. We use busybox-compatible `sleep` with float seconds (`0.05`)
// because the Alpine sh in our usual tester images accepts it. The loop emits
// a per-request log line with a known marker token so the logs-queryable
// spec can search for it.
func GenerateHTTPTraffic(kubeContext, namespace, podLabel, container, targetURL, marker string, rps, durationSec int) (string, error) {
	if rps <= 0 {
		rps = 5
	}
	if durationSec <= 0 {
		durationSec = 30
	}
	intervalMs := 1000 / rps
	totalCalls := rps * durationSec
	// `wget -q -O - <url>` is busybox-compatible. We don't care about the
	// response body — just that the call goes through. The echo line carries
	// the marker so the loop's effect is visible in pod logs as well as in
	// the workload's logs (some workloads emit their own request log).
	script := fmt.Sprintf(`set +e
for i in $(seq 1 %d); do
  wget -q -O /dev/null -T 3 %s 2>/dev/null
  echo "loadgen %s call=$i"
  sleep %s
done
echo "loadgen %s done"`,
		totalCalls,
		shellQuote(targetURL),
		marker,
		formatSleep(intervalMs),
		marker,
	)
	return KubectlExecByLabel(kubeContext, namespace, podLabel, container, "sh", "-c", script)
}

// LoadGenMarker returns a unique marker token suitable for both shell loops
// and OpenSearch search phrases. Suites should call this once per spec and
// keep the value for the polling assertion.
func LoadGenMarker(prefix string) string {
	return prefix + "-" + RandSuffix(8)
}

// formatSleep emits a busybox-`sleep`-compatible argument (seconds or
// fractional seconds). Sub-second sleeps work on alpine's busybox and the
// gitea image's busybox-replacement, both of which the e2e suites use.
func formatSleep(intervalMs int) string {
	if intervalMs >= 1000 {
		return strconv.Itoa(intervalMs / 1000)
	}
	// e.g. 200ms -> "0.2"
	return fmt.Sprintf("0.%03d", intervalMs)
}

// shellQuote single-quotes a string for embedding inside `sh -c`. URLs in
// the suites are constructed by the helper and don't contain quotes, so a
// minimal escape is sufficient.
func shellQuote(s string) string {
	return "'" + s + "'"
}
