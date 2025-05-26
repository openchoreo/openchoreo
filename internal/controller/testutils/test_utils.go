// Copyright 2025 OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package testutils

import (
	"context"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func CreateAndReconcileResource(ctx context.Context, k8sClient client.Client, resource client.Object,
	reconciler reconcile.Reconciler, namespacedName types.NamespacedName) {
	CreateAndReconcileResourceWithCycles(ctx, k8sClient, resource, reconciler, namespacedName, 1)
}

func CreateAndReconcileResourceWithCycles(ctx context.Context, k8sClient client.Client, resource client.Object,
	reconciler reconcile.Reconciler, namespacedName types.NamespacedName, reconcileCycles int) {
	err := k8sClient.Get(ctx, namespacedName, resource)
	if err != nil && errors.IsNotFound(err) {
		Expect(k8sClient.Create(ctx, resource)).To(Succeed())
	}
	for i := 0; i < reconcileCycles; i++ {
		_, err = reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: namespacedName,
		})
		Expect(err).ShouldNot(HaveOccurred())
	}
}

func DeleteResource(ctx context.Context, k8sClient client.Client, resource client.Object,
	namespacedName types.NamespacedName) {
	err := k8sClient.Get(ctx, namespacedName, resource)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
}
