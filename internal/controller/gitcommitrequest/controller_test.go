// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package gitcommitrequest

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("GitCommitRequest Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		gitcommitrequest := &openchoreov1alpha1.GitCommitRequest{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind GitCommitRequest")
			err := k8sClient.Get(ctx, typeNamespacedName, gitcommitrequest)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.GitCommitRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.GitCommitRequestSpec{
						RepoURL: "https://github.com/test/repo.git",
						Branch:  "main",
						Message: "Test commit",
						Files: []openchoreov1alpha1.FileEdit{
							{
								Path:    "test.txt",
								Content: "test content",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &openchoreov1alpha1.GitCommitRequest{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance GitCommitRequest")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
	})
})
