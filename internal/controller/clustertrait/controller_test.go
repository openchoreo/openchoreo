// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package clustertrait

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("ClusterTrait Controller", func() {
	const timeout = time.Second * 10
	const interval = time.Millisecond * 500

	// Helper to create a valid ClusterTrait with a unique name
	createClusterTrait := func(name string, spec openchoreov1alpha1.ClusterTraitSpec) *openchoreov1alpha1.ClusterTrait {
		return &openchoreov1alpha1.ClusterTrait{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: spec,
		}
	}

	// Helper to clean up a ClusterTrait resource if it exists
	cleanupClusterTrait := func(name string) {
		ct := &openchoreov1alpha1.ClusterTrait{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ct)
		if err == nil {
			Expect(k8sClient.Delete(ctx, ct)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, ct)
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		}
	}

	Context("Basic Reconciliation", func() {
		const ctName = "test-reconcile-basic"

		AfterEach(func() {
			cleanupClusterTrait(ctName)
		})

		It("should successfully reconcile a ClusterTrait with creates", func() {
			ct := createClusterTrait(ctName, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "dataplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test-config"},"data":{"key":"value"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: ctName},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: ctName}, ct)
			}, timeout, interval).Should(Succeed())
		})

		It("should setup with manager successfully", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Metrics: metricsserver.Options{
					BindAddress: "0", // disable metrics server
				},
			})
			Expect(err).NotTo(HaveOccurred())

			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			Expect(reconciler.SetupWithManager(mgr)).To(Succeed())
		})

		It("should return no error for a non-existent ClusterTrait", func() {
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "non-existent-clustertrait"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("CRUD Operations", func() {
		const ctName = "test-crud"

		AfterEach(func() {
			cleanupClusterTrait(ctName)
		})

		It("should create, read, update, and delete a ClusterTrait", func() {
			By("Creating a ClusterTrait")
			ct := createClusterTrait(ctName, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "dataplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			By("Reading the ClusterTrait")
			fetched := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ctName}, fetched)).To(Succeed())
			Expect(fetched.Spec.Creates).To(HaveLen(1))
			Expect(fetched.Spec.Creates[0].TargetPlane).To(Equal("dataplane"))

			By("Updating the ClusterTrait")
			fetched.Spec.Patches = []openchoreov1alpha1.TraitPatch{
				{
					Target: openchoreov1alpha1.PatchTarget{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					Operations: []openchoreov1alpha1.JSONPatchOperation{
						{
							Op:   "add",
							Path: "/spec/template/spec/volumes/-",
							Value: &runtime.RawExtension{
								Raw: []byte(`{"name":"data"}`),
							},
						},
					},
				},
			}
			Expect(k8sClient.Update(ctx, fetched)).To(Succeed())

			By("Verifying the update")
			updated := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ctName}, updated)).To(Succeed())
			Expect(updated.Spec.Patches).To(HaveLen(1))
			Expect(updated.Spec.Patches[0].Target.Kind).To(Equal("Deployment"))

			By("Deleting the ClusterTrait")
			Expect(k8sClient.Delete(ctx, updated)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: ctName}, &openchoreov1alpha1.ClusterTrait{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("Valid Spec Variations", func() {
		It("should accept a ClusterTrait with empty spec", func() {
			const name = "test-empty-spec"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with only schema", func() {
			const name = "test-schema-only"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Schema: openchoreov1alpha1.TraitSchema{
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"mountPath": "string"}`),
					},
					EnvOverrides: &runtime.RawExtension{
						Raw: []byte(`{"size": "string"}`),
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with only creates", func() {
			const name = "test-creates-only"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "dataplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with only patches", func() {
			const name = "test-patches-only"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/trait",
								Value: &runtime.RawExtension{
									Raw: []byte(`"applied"`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with schema, creates, and patches combined", func() {
			const name = "test-combined"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Schema: openchoreov1alpha1.TraitSchema{
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"volumeName": "string", "mountPath": "string"}`),
					},
				},
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "dataplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"name":"test-pvc"}}`),
						},
					},
				},
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/spec/template/spec/volumes/-",
								Value: &runtime.RawExtension{
									Raw: []byte(`{"name":"data"}`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with observabilityplane targetPlane in creates", func() {
			const name = "test-obs-plane-creates"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "observabilityplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"obs-config"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept a ClusterTrait with observabilityplane targetPlane in patches", func() {
			const name = "test-obs-plane-patches"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "",
							Version: "v1",
							Kind:    "Service",
						},
						TargetPlane: "observabilityplane",
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/monitored",
								Value: &runtime.RawExtension{
									Raw: []byte(`"true"`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept creates with valid forEach and var", func() {
			const name = "test-foreach-valid"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						ForEach: "${parameters.volumes}",
						Var:     "volume",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept patches with valid forEach and var", func() {
			const name = "test-patch-foreach-valid"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						ForEach: "${parameters.ports}",
						Var:     "port",
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "",
							Version: "v1",
							Kind:    "Service",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/spec/ports/-",
								Value: &runtime.RawExtension{
									Raw: []byte(`{"port": 8080}`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept creates with valid includeWhen expression", func() {
			const name = "test-includewhen"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						IncludeWhen: "${parameters.enableMetrics}",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"metrics-config"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept patches with all JSONPatch operation types", func() {
			const name = "test-all-ops"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/new-label",
								Value: &runtime.RawExtension{
									Raw: []byte(`"added"`),
								},
							},
							{
								Op:   "replace",
								Path: "/spec/replicas",
								Value: &runtime.RawExtension{
									Raw: []byte(`3`),
								},
							},
							{
								Op:   "remove",
								Path: "/metadata/annotations/old-annotation",
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should accept creates with schema types definitions", func() {
			const name = "test-schema-types"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Schema: openchoreov1alpha1.TraitSchema{
					Types: &runtime.RawExtension{
						Raw: []byte(`{"VolumeConfig": {"name": "string", "size": "string"}}`),
					},
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"volume": "VolumeConfig"}`),
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should default targetPlane to dataplane when not specified in creates", func() {
			const name = "test-default-targetplane"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			fetched := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, fetched)).To(Succeed())
			Expect(fetched.Spec.Creates[0].TargetPlane).To(Equal("dataplane"))
		})

		It("should default targetPlane to dataplane when not specified in patches", func() {
			const name = "test-default-targetplane-patch"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			fetched := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, fetched)).To(Succeed())
			Expect(fetched.Spec.Patches[0].TargetPlane).To(Equal("dataplane"))
		})
	})

	Context("CRD Validation - Creates", func() {
		It("should reject creates with forEach but no var", func() {
			const name = "test-foreach-no-var"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						ForEach: "${parameters.volumes}",
						// Var is intentionally missing
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("var is required when forEach is specified"))
		})

		It("should reject creates with invalid targetPlane value", func() {
			const name = "test-invalid-targetplane"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						TargetPlane: "invalidplane",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should reject creates with forEach not matching pattern", func() {
			const name = "test-foreach-bad-pattern"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						ForEach: "not-a-cel-expression",
						Var:     "item",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})

		It("should reject creates with includeWhen not matching pattern", func() {
			const name = "test-includewhen-bad-pattern"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						IncludeWhen: "not-a-cel-expression",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})

		It("should reject creates with var not matching identifier pattern", func() {
			const name = "test-var-bad-pattern"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						ForEach: "${parameters.items}",
						Var:     "123invalid",
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("CRD Validation - Patches", func() {
		It("should reject patches with forEach but no var", func() {
			const name = "test-patch-foreach-no-var"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						ForEach: "${parameters.ports}",
						// Var is intentionally missing
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/spec/ports/-",
								Value: &runtime.RawExtension{
									Raw: []byte(`{"port": 8080}`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("var is required when forEach is specified"))
		})

		It("should reject patches with invalid targetPlane value", func() {
			const name = "test-patch-invalid-targetplane"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						TargetPlane: "buildplane",
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should reject patches with invalid operation type", func() {
			const name = "test-patch-invalid-op"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "move",
								Path: "/spec/replicas",
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
		})

		It("should reject patches with empty operations list", func() {
			const name = "test-patch-empty-ops"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("operations"))
		})

		It("should reject patches with empty version in target", func() {
			const name = "test-patch-empty-version"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})

		It("should reject patches with empty kind in target", func() {
			const name = "test-patch-empty-kind"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})

		It("should reject patches with forEach not matching pattern", func() {
			const name = "test-patch-foreach-bad-pattern"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						ForEach: "invalid-expression",
						Var:     "item",
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})

		It("should accept patches with where filter expression", func() {
			const name = "test-patch-where"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
							Where:   "${resource.metadata.name.endsWith('-app')}",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/filtered",
								Value: &runtime.RawExtension{
									Raw: []byte(`"true"`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())
		})

		It("should reject patches with where not matching pattern", func() {
			const name = "test-patch-where-bad"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
							Where:   "not-a-cel-expression",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/filtered",
								Value: &runtime.RawExtension{
									Raw: []byte(`"true"`),
								},
							},
						},
					},
				},
			})
			err := k8sClient.Create(ctx, ct)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Cluster-Scoped Behavior", func() {
		It("should be accessible without namespace", func() {
			const name = "test-cluster-scoped"
			defer cleanupClusterTrait(name)

			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			fetched := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, fetched)).To(Succeed())
			Expect(fetched.Namespace).To(BeEmpty())
		})

		It("should support multiple ClusterTrait resources", func() {
			names := []string{"test-multi-1", "test-multi-2", "test-multi-3"}
			for _, name := range names {
				defer cleanupClusterTrait(name)
			}

			for i, name := range names {
				ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
					Creates: []openchoreov1alpha1.TraitCreate{
						{
							Template: &runtime.RawExtension{
								Raw: []byte(fmt.Sprintf(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"config-%d"}}`, i)),
							},
						},
					},
				})
				Expect(k8sClient.Create(ctx, ct)).To(Succeed())
			}

			ctList := &openchoreov1alpha1.ClusterTraitList{}
			Expect(k8sClient.List(ctx, ctList)).To(Succeed())
			// At least our 3 resources should exist
			Expect(len(ctList.Items)).To(BeNumerically(">=", 3))
		})
	})

	Context("ClusterTraitSpec has no Validations field", func() {
		It("should confirm ClusterTraitSpec does not include Validations (unlike TraitSpec)", func() {
			const name = "test-no-validations-field"
			defer cleanupClusterTrait(name)

			// Create a ClusterTrait with all three available fields
			ct := createClusterTrait(name, openchoreov1alpha1.ClusterTraitSpec{
				Schema: openchoreov1alpha1.TraitSchema{
					Parameters: &runtime.RawExtension{
						Raw: []byte(`{"key": "string"}`),
					},
				},
				Creates: []openchoreov1alpha1.TraitCreate{
					{
						Template: &runtime.RawExtension{
							Raw: []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`),
						},
					},
				},
				Patches: []openchoreov1alpha1.TraitPatch{
					{
						Target: openchoreov1alpha1.PatchTarget{
							Group:   "apps",
							Version: "v1",
							Kind:    "Deployment",
						},
						Operations: []openchoreov1alpha1.JSONPatchOperation{
							{
								Op:   "add",
								Path: "/metadata/labels/x",
								Value: &runtime.RawExtension{
									Raw: []byte(`"y"`),
								},
							},
						},
					},
				},
			})
			Expect(k8sClient.Create(ctx, ct)).To(Succeed())

			fetched := &openchoreov1alpha1.ClusterTrait{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, fetched)).To(Succeed())
			Expect(fetched.Spec.Schema.Parameters).NotTo(BeNil())
			Expect(fetched.Spec.Creates).To(HaveLen(1))
			Expect(fetched.Spec.Patches).To(HaveLen(1))
		})
	})
})
