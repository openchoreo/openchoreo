// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workload

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Workload Webhook", func() {
	const testNamespace = "default"

	var (
		validator Validator
	)

	BeforeEach(func() {
		validator = Validator{Client: k8sClient}
	})

	Context("When creating a Workload", func() {
		It("Should admit creation when no other Workload exists for the Component", func() {
			workload := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload-create-admit",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-create-admit",
						ComponentName: "component-create-admit",
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, workload)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should reject creation when a Workload already exists for the same Component", func() {
			// Create the first workload in the cluster
			existing := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-workload-dup",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-dup",
						ComponentName: "component-dup",
					},
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			// Try to create a second workload for the same component
			duplicate := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "duplicate-workload-dup",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-dup",
						ComponentName: "component-dup",
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, duplicate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("a workload"))
			Expect(err.Error()).To(ContainSubstring("already exists for component"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
		})

		It("Should admit creation for a different Component in the same Project", func() {
			// Create the first workload
			existing := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-workload-diff",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-diff",
						ComponentName: "component-a",
					},
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			// Create a workload for a different component in the same project
			different := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "different-workload-diff",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-diff",
						ComponentName: "component-b",
					},
				},
			}

			_, err := validator.ValidateCreate(ctx, different)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
		})
	})

	Context("When updating a Workload", func() {
		It("Should admit update of non-owner fields", func() {
			// Create the workload
			existing := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "existing-workload-update",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-update",
						ComponentName: "component-update",
					},
				},
			}
			Expect(k8sClient.Create(ctx, existing)).To(Succeed())

			// Update the workload (non-owner fields)
			updated := existing.DeepCopy()
			updated.Spec.Containers = map[string]openchoreodevv1alpha1.Container{
				"main": {Image: "nginx:latest"},
			}

			_, err := validator.ValidateUpdate(context.Background(), existing, updated)
			Expect(err).ToNot(HaveOccurred())

			// Cleanup
			Expect(k8sClient.Delete(ctx, existing)).To(Succeed())
		})
	})

	Context("When deleting a Workload", func() {
		It("Should admit deletion", func() {
			workload := &openchoreodevv1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload-delete",
					Namespace: testNamespace,
				},
				Spec: openchoreodevv1alpha1.WorkloadSpec{
					Owner: openchoreodevv1alpha1.WorkloadOwner{
						ProjectName:   "project-delete",
						ComponentName: "component-delete",
					},
				},
			}

			_, err := validator.ValidateDelete(context.Background(), workload)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
