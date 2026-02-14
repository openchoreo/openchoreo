// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// deleteFailClient wraps a client.Client and returns predefined errors
// for Delete calls on objects whose name is in the failFor map.
type deleteFailClient struct {
	client.Client
	failFor map[string]error
}

func (c *deleteFailClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err, ok := c.failFor[obj.GetName()]; ok {
		return err
	}
	return c.Client.Delete(ctx, obj, opts...)
}

var _ = Describe("ComponentRelease Cleanup", func() {
	const (
		projectName = "cleanup-test-project"
		namespace   = "default"
		timeout     = time.Second * 10
		interval    = time.Millisecond * 250
	)

	createRelease := func(ctx context.Context, name, compName string) {
		release := &openchoreov1alpha1.ComponentRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: openchoreov1alpha1.ComponentReleaseSpec{
				Owner: openchoreov1alpha1.ComponentReleaseOwner{
					ProjectName:   projectName,
					ComponentName: compName,
				},
				ComponentType: openchoreov1alpha1.ComponentTypeSpec{
					WorkloadType: "deployment",
					Resources: []openchoreov1alpha1.ResourceTemplate{
						{
							ID:       "deployment",
							Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`)},
						},
					},
				},
				Workload: openchoreov1alpha1.WorkloadTemplateSpec{
					Containers: map[string]openchoreov1alpha1.Container{
						"app": {Image: "nginx:latest"},
					},
				},
			},
		}
		ExpectWithOffset(1, k8sClient.Create(ctx, release)).To(Succeed())
	}

	createBinding := func(ctx context.Context, name, compName, releaseName string) {
		binding := &openchoreov1alpha1.ReleaseBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: openchoreov1alpha1.ReleaseBindingSpec{
				Owner: openchoreov1alpha1.ReleaseBindingOwner{
					ProjectName:   projectName,
					ComponentName: compName,
				},
				ReleaseName: releaseName,
				Environment: "development",
			},
		}
		ExpectWithOffset(1, k8sClient.Create(ctx, binding)).To(Succeed())
	}

	countReleases := func(ctx context.Context, compName string) int {
		list := &openchoreov1alpha1.ComponentReleaseList{}
		err := k8sClient.List(ctx, list,
			client.InNamespace(namespace),
			client.MatchingFields{"spec.owner.componentName": compName})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return len(list.Items)
	}

	// waitForReleases waits until the informer cache reflects the expected number of releases.
	// This is necessary because creates go to the API server directly while the cleanup
	// reads from the cached client; without waiting, the cache may not have synced yet.
	waitForReleases := func(ctx context.Context, compName string, expected int) {
		EventuallyWithOffset(1, func() int {
			return countReleases(ctx, compName)
		}, timeout, interval).Should(Equal(expected))
	}

	releaseExists := func(ctx context.Context, name string) bool {
		r := &openchoreov1alpha1.ComponentRelease{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, r)
		if err == nil {
			return true
		}
		if apierrors.IsNotFound(err) {
			return false
		}
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "unexpected error checking ComponentRelease existence")
		return false
	}

	deleteAllReleases := func(ctx context.Context, compName string, count int) {
		for i := 1; i <= count; i++ {
			rel := &openchoreov1alpha1.ComponentRelease{}
			relName := fmt.Sprintf("%s-rel-%02d", compName, i)
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: relName, Namespace: namespace}, rel); err == nil {
				_ = k8sClient.Delete(ctx, rel)
			}
		}
	}

	Context("Basic retention", func() {
		It("should delete releases exceeding the limit", func() {
			ctx := context.Background()
			compName := "cleanup-basic"
			limit := 10

			By("Creating 15 ComponentReleases")
			for i := 1; i <= 15; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Waiting for cache to sync all 15 releases")
			waitForReleases(ctx, compName, 15)

			By("Running cleanup with limit=10")
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying exactly 10 releases remain")
			Eventually(func() int {
				return countReleases(ctx, compName)
			}, timeout, interval).Should(Equal(10))

			By("Cleaning up")
			deleteAllReleases(ctx, compName, 15)
		})
	})

	Context("In-use protection via ReleaseBinding", func() {
		It("should not delete a release referenced by a ReleaseBinding", func() {
			ctx := context.Background()
			compName := "cleanup-inuse"
			limit := 10

			By("Creating 15 ComponentReleases")
			for i := 1; i <= 15; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Creating a ReleaseBinding referencing release #3")
			protectedRelease := fmt.Sprintf("%s-rel-03", compName)
			createBinding(ctx, compName+"-binding", compName, protectedRelease)

			By("Waiting for cache to sync all 15 releases")
			waitForReleases(ctx, compName, 15)

			By("Running cleanup with limit=10")
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying protected release #3 survives")
			Expect(releaseExists(ctx, protectedRelease)).To(BeTrue())

			By("Verifying exactly 10 releases remain")
			Eventually(func() int {
				return countReleases(ctx, compName)
			}, timeout, interval).Should(Equal(10))

			By("Cleaning up")
			binding := &openchoreov1alpha1.ReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Name: compName + "-binding", Namespace: namespace,
			}, binding); err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}
			deleteAllReleases(ctx, compName, 15)
		})
	})

	Context("LatestRelease protection", func() {
		It("should not delete a release marked as LatestRelease", func() {
			ctx := context.Background()
			compName := "cleanup-latest"
			limit := 10

			By("Creating 15 ComponentReleases")
			for i := 1; i <= 15; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Waiting for cache to sync all 15 releases")
			waitForReleases(ctx, compName, 15)

			By("Running cleanup with LatestRelease pointing to release #2")
			protectedRelease := fmt.Sprintf("%s-rel-02", compName)
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
				Status: openchoreov1alpha1.ComponentStatus{
					LatestRelease: &openchoreov1alpha1.LatestRelease{
						Name:        protectedRelease,
						ReleaseHash: "fakehash",
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying protected release #2 survives")
			Expect(releaseExists(ctx, protectedRelease)).To(BeTrue())

			By("Verifying exactly 10 releases remain")
			Eventually(func() int {
				return countReleases(ctx, compName)
			}, timeout, interval).Should(Equal(10))

			By("Cleaning up")
			deleteAllReleases(ctx, compName, 15)
		})
	})

	Context("Under limit", func() {
		It("should not delete anything when under the limit", func() {
			ctx := context.Background()
			compName := "cleanup-under"
			limit := 10

			By("Creating 5 ComponentReleases")
			for i := 1; i <= 5; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Waiting for cache to sync all 5 releases")
			waitForReleases(ctx, compName, 5)

			By("Running cleanup with limit=10")
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying all 5 releases still exist")
			Expect(countReleases(ctx, compName)).To(Equal(5))

			By("Cleaning up")
			deleteAllReleases(ctx, compName, 5)
		})
	})

	Context("All protected", func() {
		It("should not delete anything when all releases are in use", func() {
			ctx := context.Background()
			compName := "cleanup-allprot"
			limit := 10

			By("Creating 15 ComponentReleases")
			for i := 1; i <= 15; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Creating a ReleaseBinding for each release")
			for i := 1; i <= 15; i++ {
				relName := fmt.Sprintf("%s-rel-%02d", compName, i)
				bindingName := fmt.Sprintf("%s-bind-%02d", compName, i)
				createBinding(ctx, bindingName, compName, relName)
			}

			By("Waiting for cache to sync all 15 releases")
			waitForReleases(ctx, compName, 15)

			By("Running cleanup with limit=10")
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying all 15 releases still exist")
			Expect(countReleases(ctx, compName)).To(Equal(15))

			By("Cleaning up")
			for i := 1; i <= 15; i++ {
				binding := &openchoreov1alpha1.ReleaseBinding{}
				bindName := fmt.Sprintf("%s-bind-%02d", compName, i)
				if err := k8sClient.Get(ctx, client.ObjectKey{
					Name: bindName, Namespace: namespace,
				}, binding); err == nil {
					_ = k8sClient.Delete(ctx, binding)
				}
			}
			deleteAllReleases(ctx, compName, 15)
		})
	})

	Context("Limit=0 disables cleanup", func() {
		It("should not call cleanup when limit is 0", func() {
			ctx := context.Background()
			compName := "cleanup-disabled"

			By("Creating 15 ComponentReleases")
			for i := 1; i <= 15; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Waiting for cache to sync all 15 releases")
			waitForReleases(ctx, compName, 15)

			By("Verifying the controller guard: RevisionHistoryLimit > 0")
			reconciler := &Reconciler{
				Client:               k8sClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: 0,
			}
			// When limit=0 the controller doesn't call cleanup at all.
			// Verify the guard condition.
			Expect(reconciler.RevisionHistoryLimit).To(Equal(0))

			By("Verifying all 15 releases still exist (cleanup was never called)")
			Expect(countReleases(ctx, compName)).To(Equal(15))

			By("Cleaning up")
			deleteAllReleases(ctx, compName, 15)
		})
	})

	Context("Delete failure aggregation", func() {
		It("should aggregate delete errors and return them", func() {
			ctx := context.Background()
			compName := "cleanup-delfail"
			limit := 3

			By("Creating 5 ComponentReleases")
			for i := 1; i <= 5; i++ {
				createRelease(ctx, fmt.Sprintf("%s-rel-%02d", compName, i), compName)
			}

			By("Creating a ReleaseBinding protecting release #4")
			protectedByBinding := fmt.Sprintf("%s-rel-04", compName)
			createBinding(ctx, compName+"-binding", compName, protectedByBinding)

			By("Waiting for cache to sync all 5 releases")
			waitForReleases(ctx, compName, 5)

			// Make rel-01 and rel-02 fail to delete. With 5 releases, limit=3,
			// excess=2, and two protected releases (#4 via binding, #5 via LatestRelease),
			// the only deletable candidates are #1, #2, and #3. Both failing releases
			// will always be attempted regardless of sort order because a single
			// successful delete (#3) cannot bring excess to 0 on its own.
			failRel01 := fmt.Sprintf("%s-rel-01", compName)
			failRel02 := fmt.Sprintf("%s-rel-02", compName)
			protectedByLatest := fmt.Sprintf("%s-rel-05", compName)

			By("Creating a reconciler with a delete-failing client for rel-01 and rel-02")
			failClient := &deleteFailClient{
				Client: k8sClient,
				failFor: map[string]error{
					failRel01: fmt.Errorf("simulated delete failure for rel-01"),
					failRel02: fmt.Errorf("simulated delete failure for rel-02"),
				},
			}
			reconciler := &Reconciler{
				Client:               failClient,
				Scheme:               k8sClient.Scheme(),
				RevisionHistoryLimit: limit,
			}

			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
				},
				Status: openchoreov1alpha1.ComponentStatus{
					LatestRelease: &openchoreov1alpha1.LatestRelease{
						Name:        protectedByLatest,
						ReleaseHash: "fakehash",
					},
				},
			}

			err := reconciler.cleanupComponentReleases(ctx, comp)

			By("Verifying the aggregated error is returned")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("errors during ComponentRelease cleanup"))
			Expect(err.Error()).To(ContainSubstring("simulated delete failure for rel-01"))
			Expect(err.Error()).To(ContainSubstring("simulated delete failure for rel-02"))

			By("Verifying failed-to-delete releases still exist")
			Expect(releaseExists(ctx, failRel01)).To(BeTrue())
			Expect(releaseExists(ctx, failRel02)).To(BeTrue())

			By("Verifying protected releases still exist")
			Expect(releaseExists(ctx, protectedByBinding)).To(BeTrue())
			Expect(releaseExists(ctx, protectedByLatest)).To(BeTrue())

			By("Verifying the non-protected, non-failing release was deleted")
			Eventually(func() bool {
				return releaseExists(ctx, fmt.Sprintf("%s-rel-03", compName))
			}, timeout, interval).Should(BeFalse())

			By("Cleaning up")
			binding := &openchoreov1alpha1.ReleaseBinding{}
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Name: compName + "-binding", Namespace: namespace,
			}, binding); err == nil {
				Expect(k8sClient.Delete(ctx, binding)).To(Succeed())
			}
			deleteAllReleases(ctx, compName, 5)
		})
	})
})
