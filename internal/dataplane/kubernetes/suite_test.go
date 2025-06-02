// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDataPlaneKubernetes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Plane Kubernetes Suite")
}
