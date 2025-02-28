/*
 * Copyright (c) 2025, WSO2 Inc. (http://www.wso2.org) All Rights Reserved.
 *
 * WSO2 Inc. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

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
