// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releasebinding

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	crconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	componentpipeline "github.com/openchoreo/openchoreo/internal/pipeline/component"
)

// The rendering pipeline carries the CEL cost limit, so where the pipeline comes from is part
// of the guard. cmd/main.go injects one; SetupWithManager is the other way a Reconciler is
// wired up, and it has to build one from the configured limit rather than leave the field nil.
// The workflowrun reconciler already does this, which is what makes the omission here a gap
// rather than a design choice.
var _ = Describe("ReleaseBinding controller — SetupWithManager", func() {
	newManager := func() ctrl.Manager {
		GinkgoHelper()
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:  k8sClient.Scheme(),
			Metrics: metricsserver.Options{BindAddress: "0"},
			// Each spec builds its own manager, and controller-runtime rejects a second
			// controller registered under a name it has already seen.
			Controller: crconfig.Controller{SkipNameValidation: ptr.To(true)},
		})
		Expect(err).NotTo(HaveOccurred())
		return mgr
	}

	It("builds a pipeline when none was injected", func() {
		r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), CELCostLimit: 12_345}
		Expect(r.SetupWithManager(newManager())).To(Succeed())
		Expect(r.Pipeline).NotTo(BeNil())
	})

	It("leaves an injected pipeline alone", func() {
		injected := componentpipeline.NewPipeline(componentpipeline.WithCostLimit(999))
		r := &Reconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Pipeline: injected, CELCostLimit: 12_345}
		Expect(r.SetupWithManager(newManager())).To(Succeed())
		Expect(r.Pipeline).To(BeIdenticalTo(injected))
	})
})
