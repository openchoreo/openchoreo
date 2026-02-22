// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package component

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	ocLabels "github.com/openchoreo/openchoreo/internal/labels"
)

var _ = Describe("Component Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		component := &openchoreov1alpha1.Component{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Component")
			err := k8sClient.Get(ctx, typeNamespacedName, component)
			if err != nil && errors.IsNotFound(err) {
				resource := &openchoreov1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: openchoreov1alpha1.ComponentSpec{
						Owner: openchoreov1alpha1.ComponentOwner{
							ProjectName: "test-project",
						},
						ComponentType: openchoreov1alpha1.ComponentTypeRef{
							Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
							Name: "deployment/service",
						},
						Workflow: &openchoreov1alpha1.WorkflowRunConfig{Name: "test-workflow"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &openchoreov1alpha1.Component{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup the specific resource instance Component")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

var _ = Describe("Component Controller with ComponentType", func() {
	Context("When reconciling a Component with componentType ref", func() {
		const (
			componentName     = "ct-ref-test-component"
			componentTypeName = "ct-ref-test-ct"
			namespace         = "default"
		)

		ctx := context.Background()

		componentNamespacedName := types.NamespacedName{
			Name:      componentName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			By("Creating the ComponentType resource")
			ct := &openchoreov1alpha1.ComponentType{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: componentTypeName, Namespace: namespace}, ct)
			if err != nil && errors.IsNotFound(err) {
				ct = &openchoreov1alpha1.ComponentType{
					ObjectMeta: metav1.ObjectMeta{
						Name:      componentTypeName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.ComponentTypeSpec{
						WorkloadType: "deployment",
						Resources: []openchoreov1alpha1.ResourceTemplate{
							{
								ID:       "deployment",
								Template: &runtime.RawExtension{Raw: []byte(`{"apiVersion":"apps/v1","kind":"Deployment"}`)},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, ct)).To(Succeed())
			}

			By("Creating the Component with componentType ref")
			comp := &openchoreov1alpha1.Component{}
			err = k8sClient.Get(ctx, componentNamespacedName, comp)
			if err != nil && errors.IsNotFound(err) {
				comp = &openchoreov1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      componentName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.ComponentSpec{
						Owner: openchoreov1alpha1.ComponentOwner{
							ProjectName: "test-project",
						},
						ComponentType: openchoreov1alpha1.ComponentTypeRef{
							Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
							Name: "deployment/" + componentTypeName,
						},
					},
				}
				Expect(k8sClient.Create(ctx, comp)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleaning up Component")
			comp := &openchoreov1alpha1.Component{}
			if err := k8sClient.Get(ctx, componentNamespacedName, comp); err == nil {
				Expect(k8sClient.Delete(ctx, comp)).To(Succeed())
			}

			By("Cleaning up ComponentType")
			ct := &openchoreov1alpha1.ComponentType{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: componentTypeName, Namespace: namespace}, ct); err == nil {
				Expect(k8sClient.Delete(ctx, ct)).To(Succeed())
			}
		})

		It("should reconcile without error when ComponentType exists", func() {
			By("Reconciling the Component with ComponentType ref")
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a Component referencing a non-existent ComponentType", func() {
		const (
			componentName = "ct-ref-missing-component"
			namespace     = "default"
		)

		ctx := context.Background()

		componentNamespacedName := types.NamespacedName{
			Name:      componentName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			By("Creating the Component with a ref to non-existent ComponentType")
			comp := &openchoreov1alpha1.Component{}
			err := k8sClient.Get(ctx, componentNamespacedName, comp)
			if err != nil && errors.IsNotFound(err) {
				comp = &openchoreov1alpha1.Component{
					ObjectMeta: metav1.ObjectMeta{
						Name:      componentName,
						Namespace: namespace,
					},
					Spec: openchoreov1alpha1.ComponentSpec{
						Owner: openchoreov1alpha1.ComponentOwner{
							ProjectName: "test-project",
						},
						ComponentType: openchoreov1alpha1.ComponentTypeRef{
							Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
							Name: "deployment/nonexistent-ct",
						},
					},
				}
				Expect(k8sClient.Create(ctx, comp)).To(Succeed())
			}
		})

		AfterEach(func() {
			comp := &openchoreov1alpha1.Component{}
			if err := k8sClient.Get(ctx, componentNamespacedName, comp); err == nil {
				Expect(k8sClient.Delete(ctx, comp)).To(Succeed())
			}
		})

		It("should reconcile without error and set condition when ComponentType not found", func() {
			controllerReconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("First reconcile adds the finalizer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Second reconcile performs ComponentType validation")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: componentNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Ready condition is set to False")
			comp := &openchoreov1alpha1.Component{}
			Expect(k8sClient.Get(ctx, componentNamespacedName, comp)).To(Succeed())

			var readyCondition *metav1.Condition
			for i := range comp.Status.Conditions {
				if comp.Status.Conditions[i].Type == string(ConditionReady) {
					readyCondition = &comp.Status.Conditions[i]
					break
				}
			}
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal(string(ReasonComponentTypeNotFound)))
		})
	})
})

var _ = Describe("Component Controller Finalization", func() {
	const (
		projectName   = "finalize-test-project"
		componentName = "finalize-test-component"
		namespace     = "default"
		timeout       = time.Second * 10
		interval      = time.Millisecond * 250
	)

	var (
		ctx        context.Context
		reconciler *Reconciler
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &Reconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("When deleting a Component with no owned resources", func() {
		It("should finalize and delete the Component immediately", func() {
			compName := componentName + "-no-owned"
			compNamespacedName := types.NamespacedName{Name: compName, Namespace: namespace}

			By("Creating a Component")
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
					ComponentType: openchoreov1alpha1.ComponentTypeRef{
						Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
						Name: "deployment/service",
					},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())

			By("Reconciling to add finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer is added")
			Eventually(func() bool {
				updated := &openchoreov1alpha1.Component{}
				if err := k8sClient.Get(ctx, compNamespacedName, updated); err != nil {
					return false
				}
				for _, f := range updated.Finalizers {
					if f == ComponentFinalizer {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Deleting the Component")
			Expect(k8sClient.Delete(ctx, comp)).To(Succeed())

			By("Verifying Component is marked for deletion")
			Eventually(func() bool {
				updated := &openchoreov1alpha1.Component{}
				if err := k8sClient.Get(ctx, compNamespacedName, updated); err != nil {
					return false
				}
				return !updated.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			By("Reconciling to set Finalizing condition")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling to complete finalization")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Component is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, compNamespacedName, &openchoreov1alpha1.Component{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When deleting a Component with all owned resource types", func() {
		It("should wait for all children to be deleted before removing finalizer", func() {
			compName := componentName + "-full-owned"
			releaseName := compName + "-release-v1"
			bindingName := compName + "-binding-dev"
			workloadName := compName + "-workload"
			workflowRunName := compName + "-build-01"
			compNamespacedName := types.NamespacedName{Name: compName, Namespace: namespace}
			releaseNamespacedName := types.NamespacedName{Name: releaseName, Namespace: namespace}
			bindingNamespacedName := types.NamespacedName{Name: bindingName, Namespace: namespace}
			workloadNamespacedName := types.NamespacedName{Name: workloadName, Namespace: namespace}
			workflowRunNamespacedName := types.NamespacedName{Name: workflowRunName, Namespace: namespace}

			By("Creating a Component")
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
					ComponentType: openchoreov1alpha1.ComponentTypeRef{
						Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
						Name: "deployment/service",
					},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())

			By("Creating an owned ComponentRelease")
			release := &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      releaseName,
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
					ComponentProfile: nil,
					Workload: openchoreov1alpha1.WorkloadTemplateSpec{
						Container: openchoreov1alpha1.Container{
							Image: "nginx:latest",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Creating an owned ReleaseBinding")
			binding := &openchoreov1alpha1.ReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      bindingName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ReleaseBindingSpec{
					Owner: openchoreov1alpha1.ReleaseBindingOwner{
						ProjectName:   projectName,
						ComponentName: compName,
					},
					Environment: "development",
				},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())

			By("Creating an owned Workload")
			workload := &openchoreov1alpha1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workloadName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.WorkloadSpec{
					Owner: openchoreov1alpha1.WorkloadOwner{
						ProjectName:   projectName,
						ComponentName: compName,
					},
					WorkloadTemplateSpec: openchoreov1alpha1.WorkloadTemplateSpec{
						Container: openchoreov1alpha1.Container{
							Image: "nginx:latest",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, workload)).To(Succeed())

			By("Creating an owned WorkflowRun")
			workflowRun := &openchoreov1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowRunName,
					Namespace: namespace,
					Labels: map[string]string{
						ocLabels.LabelKeyProjectName:   projectName,
						ocLabels.LabelKeyComponentName: compName,
					},
				},
				Spec: openchoreov1alpha1.WorkflowRunSpec{
					Workflow: openchoreov1alpha1.WorkflowRunConfig{
						Name: "test-workflow",
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflowRun)).To(Succeed())

			By("Reconciling to add finalizer")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying finalizer is present")
			updated := &openchoreov1alpha1.Component{}
			Expect(k8sClient.Get(ctx, compNamespacedName, updated)).To(Succeed())
			Expect(updated.Finalizers).To(ContainElement(ComponentFinalizer))

			By("Deleting the Component")
			Expect(k8sClient.Delete(ctx, comp)).To(Succeed())

			By("Verifying Component is marked for deletion but still exists due to finalizer")
			Eventually(func() bool {
				c := &openchoreov1alpha1.Component{}
				if err := k8sClient.Get(ctx, compNamespacedName, c); err != nil {
					return false
				}
				return !c.DeletionTimestamp.IsZero()
			}, timeout, interval).Should(BeTrue())

			By("First reconcile sets Finalizing condition")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Component still has finalizer while children exist")
			compDuringFinalization := &openchoreov1alpha1.Component{}
			Expect(k8sClient.Get(ctx, compNamespacedName, compDuringFinalization)).To(Succeed())
			Expect(compDuringFinalization.Finalizers).To(ContainElement(ComponentFinalizer))
			Expect(compDuringFinalization.DeletionTimestamp.IsZero()).To(BeFalse())

			By("Reconciling to trigger deletion of all children")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying result requests requeue while children exist")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Component still exists with finalizer during child deletion")
			compStillPresent := &openchoreov1alpha1.Component{}
			Expect(k8sClient.Get(ctx, compNamespacedName, compStillPresent)).To(Succeed())
			Expect(compStillPresent.Finalizers).To(ContainElement(ComponentFinalizer))

			By("Reconciling to complete finalization after all children are gone")
			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
				if err != nil {
					return false
				}
				c := &openchoreov1alpha1.Component{}
				err = k8sClient.Get(ctx, compNamespacedName, c)
				return errors.IsNotFound(err)
			}, timeout*3, interval).Should(BeTrue())

			By("Verifying all resources are deleted")
			err = k8sClient.Get(ctx, releaseNamespacedName, &openchoreov1alpha1.ComponentRelease{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, bindingNamespacedName, &openchoreov1alpha1.ReleaseBinding{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, workloadNamespacedName, &openchoreov1alpha1.Workload{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, workflowRunNamespacedName, &openchoreov1alpha1.WorkflowRun{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
			err = k8sClient.Get(ctx, compNamespacedName, &openchoreov1alpha1.Component{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})

	Context("When deleting a Component with slow-deleting child (blocked by finalizer)", func() {
		const childFinalizer = "test.openchoreo.dev/block-deletion"

		It("should wait for blocked child to finish terminating before removing Component finalizer", func() {
			compName := componentName + "-blocked-child"
			releaseName := compName + "-release-blocked"
			compNamespacedName := types.NamespacedName{Name: compName, Namespace: namespace}
			releaseNamespacedName := types.NamespacedName{Name: releaseName, Namespace: namespace}

			By("Creating a Component")
			comp := &openchoreov1alpha1.Component{
				ObjectMeta: metav1.ObjectMeta{
					Name:      compName,
					Namespace: namespace,
				},
				Spec: openchoreov1alpha1.ComponentSpec{
					Owner: openchoreov1alpha1.ComponentOwner{
						ProjectName: projectName,
					},
					ComponentType: openchoreov1alpha1.ComponentTypeRef{
						Kind: openchoreov1alpha1.ComponentTypeRefKindComponentType,
						Name: "deployment/service",
					},
				},
			}
			Expect(k8sClient.Create(ctx, comp)).To(Succeed())

			By("Creating an owned ComponentRelease with a blocking finalizer")
			release := &openchoreov1alpha1.ComponentRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:       releaseName,
					Namespace:  namespace,
					Finalizers: []string{childFinalizer},
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
					ComponentProfile: nil,
					Workload: openchoreov1alpha1.WorkloadTemplateSpec{
						Container: openchoreov1alpha1.Container{
							Image: "nginx:latest",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, release)).To(Succeed())

			By("Reconciling to add finalizer to Component")
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Deleting the Component")
			Expect(k8sClient.Delete(ctx, comp)).To(Succeed())

			By("Reconciling to set Finalizing condition")
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling to trigger child deletion")
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ComponentRelease is marked for deletion but still exists")
			Eventually(func(g Gomega) {
				blockedRelease := &openchoreov1alpha1.ComponentRelease{}
				g.Expect(k8sClient.Get(ctx, releaseNamespacedName, blockedRelease)).To(Succeed())
				g.Expect(blockedRelease.DeletionTimestamp.IsZero()).To(BeFalse())
				g.Expect(blockedRelease.Finalizers).To(ContainElement(childFinalizer))
			}, timeout, interval).Should(Succeed())

			By("Verifying controller requeues while child is blocked")
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying Component still exists with finalizer while child is terminating")
			compBlocked := &openchoreov1alpha1.Component{}
			Expect(k8sClient.Get(ctx, compNamespacedName, compBlocked)).To(Succeed())
			Expect(compBlocked.Finalizers).To(ContainElement(ComponentFinalizer))
			Expect(compBlocked.DeletionTimestamp.IsZero()).To(BeFalse())

			By("Running multiple reconciles while child is blocked - Component should not be deleted")
			for range 3 {
				result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				compStillExists := &openchoreov1alpha1.Component{}
				Expect(k8sClient.Get(ctx, compNamespacedName, compStillExists)).To(Succeed())
				Expect(compStillExists.Finalizers).To(ContainElement(ComponentFinalizer))
			}

			By("Simulating child finalizer removal (external process completes cleanup)")
			releaseToUnblock := &openchoreov1alpha1.ComponentRelease{}
			Expect(k8sClient.Get(ctx, releaseNamespacedName, releaseToUnblock)).To(Succeed())
			releaseToUnblock.Finalizers = nil
			Expect(k8sClient.Update(ctx, releaseToUnblock)).To(Succeed())

			By("Waiting for ComponentRelease to be fully deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, releaseNamespacedName, &openchoreov1alpha1.ComponentRelease{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			By("Reconciling to complete finalization")
			Eventually(func() bool {
				_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: compNamespacedName})
				if err != nil {
					return false
				}
				c := &openchoreov1alpha1.Component{}
				err = k8sClient.Get(ctx, compNamespacedName, c)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})
})
