// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/openchoreo/openchoreo/test/e2e/framework"
)

func main() {
	var kubeContext string
	flag.StringVar(&kubeContext, "e2e.kubecontext", "",
		"Kubernetes context for e2e fixture setup (required)")
	flag.Parse()

	if kubeContext == "" {
		fmt.Fprintln(os.Stderr, "--e2e.kubecontext is required")
		os.Exit(2)
	}
	if err := framework.EnsureTier3BuildSources(kubeContext); err != nil {
		fmt.Fprintf(os.Stderr, "failed to set up Tier 3 fixtures: %v\n", err)
		os.Exit(1)
	}
}
