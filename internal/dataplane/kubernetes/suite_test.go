/*
 * Copyright Open Choreo Authors
 * SPDX-License-Identifier: Apache-2.0
 * The full text of the Apache license is available in the LICENSE file at
 * the root of the repo.
 */

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
