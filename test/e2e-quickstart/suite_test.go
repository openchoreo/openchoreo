// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package quickstart

import (
	"flag"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

var (
	image   string
	version string
)

func init() {
	flag.StringVar(&image, "qs.image", "ghcr.io/openchoreo/quick-start:latest-dev",
		"Quick-start container image to test")
	flag.StringVar(&version, "qs.version", "latest-dev",
		"OpenChoreo version to pass to install.sh")
}

func TestQuickStart(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintf(GinkgoWriter, "Starting OpenChoreo Quick Start e2e suite\n")
	RunSpecs(t, "Quick Start E2E Suite")
}
