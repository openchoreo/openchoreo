// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflow

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("Workflow Controller End-to-End", func() {
	Context("When reconciling a Workflow with WorkflowDefinition", func() {
		const (
			workflowName         = "test-workflow"
			workflowDefName      = "test-workflow-definition"
			componentTypeDefName = "test-component-type"
			namespace            = "default"
			timeout              = time.Second * 10
			interval             = time.Millisecond * 250
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      workflowName,
			Namespace: namespace,
		}

		BeforeEach(func() {
			By("Creating a WorkflowDefinition with schema and template")
			workflowDef := &openchoreodevv1alpha1.WorkflowDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowDefName,
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowDefinitionSpec{
					Schema: mustMarshalRaw(map[string]any{
						"repository": map[string]any{
							"url":    "string",
							"branch": "string | default=main",
							"commit": "string | default=HEAD",
						},
						"version": "integer | default=1",
					}),
					FixedParameters: []openchoreodevv1alpha1.WorkflowParameter{
						{Name: "builder_image", Value: "gcr.io/buildpacks/builder:v1"},
						{Name: "security_scan", Value: "true"},
						{Name: "timeout", Value: "30m"},
					},
					Resource: openchoreodevv1alpha1.WorkflowResource{
						Template: mustMarshalRaw(map[string]any{
							"apiVersion": "argoproj.io/v1alpha1",
							"kind":       "Workflow",
							"metadata": map[string]any{
								"name":      "${ctx.componentName}-${ctx.uuid}",
								"namespace": "build-${ctx.orgName}",
								"labels": map[string]any{
									"openchoreo.dev/component": "${ctx.componentName}",
									"openchoreo.dev/project":   "${ctx.projectName}",
								},
							},
							"spec": map[string]any{
								"serviceAccountName": "build-bot",
								"arguments": map[string]any{
									"parameters": []any{
										map[string]any{"name": "repo-url", "value": "${schema.repository.url}"},
										map[string]any{"name": "branch", "value": "${schema.repository.branch}"},
										map[string]any{"name": "commit", "value": "${schema.repository.commit}"},
										map[string]any{"name": "version", "value": "${schema.version}"},
										map[string]any{"name": "builder-image", "value": "${fixedParameters.builder_image}"},
										map[string]any{"name": "security-scan", "value": "${fixedParameters.security_scan}"},
										map[string]any{"name": "timeout", "value": "${fixedParameters.timeout}"},
									},
								},
								"workflowTemplateRef": map[string]any{
									"name": "buildpacks-template",
								},
							},
						}),
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflowDef)).To(Succeed())

			By("Creating a ComponentTypeDefinition with parameter overrides")
			componentTypeDef := &openchoreodevv1alpha1.ComponentTypeDefinition{
				ObjectMeta: metav1.ObjectMeta{
					Name:      componentTypeDefName,
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.ComponentTypeDefinitionSpec{
					WorkloadType: "deployment",
					Resources: []openchoreodevv1alpha1.ResourceTemplate{
						{
							ID:       "deployment",
							Template: mustMarshalRaw(map[string]any{"apiVersion": "apps/v1", "kind": "Deployment"}),
						},
					},
					Build: &openchoreodevv1alpha1.ComponentTypeBuildConfig{
						AllowedTemplates: []openchoreodevv1alpha1.AllowedWorkflowTemplate{
							{
								Name: workflowDefName,
								FixedParameters: []openchoreodevv1alpha1.WorkflowParameter{
									{Name: "security_scan", Value: "false"}, // Override
									{Name: "timeout", Value: "45m"},         // Override
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, componentTypeDef)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up WorkflowDefinition")
			workflowDef := &openchoreodevv1alpha1.WorkflowDefinition{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: workflowDefName, Namespace: namespace}, workflowDef); err == nil {
				Expect(k8sClient.Delete(ctx, workflowDef)).To(Succeed())
			}

			By("Cleaning up ComponentTypeDefinition")
			componentTypeDef := &openchoreodevv1alpha1.ComponentTypeDefinition{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: componentTypeDefName, Namespace: namespace}, componentTypeDef); err == nil {
				Expect(k8sClient.Delete(ctx, componentTypeDef)).To(Succeed())
			}

			By("Cleaning up Workflow")
			workflow := &openchoreodevv1alpha1.Workflow{}
			if err := k8sClient.Get(ctx, typeNamespacedName, workflow); err == nil {
				Expect(k8sClient.Delete(ctx, workflow)).To(Succeed())
			}

			By("Cleaning up rendered Argo Workflow")
			// Find and delete the rendered workflow
			workflowList := &unstructured.UnstructuredList{}
			workflowList.SetGroupVersionKind(argoWorkflowGVK())
			if err := k8sClient.List(ctx, workflowList); err == nil {
				for _, item := range workflowList.Items {
					Expect(k8sClient.Delete(ctx, &item)).To(Succeed())
				}
			}
		})

		It("should render Argo Workflow with correct values from all sources", func() {
			By("Creating a Workflow with developer parameters")
			workflow := &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowName,
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					Owner: openchoreodevv1alpha1.WorkflowOwner{
						ProjectName:   "ecommerce",
						ComponentName: "checkout-service",
					},
					WorkflowDefinitionRef: openchoreodevv1alpha1.WorkflowDefinitionReference{
						Name:      workflowDefName,
						Namespace: namespace,
					},
					Parameters: mustMarshalRaw(map[string]any{
						"repository": map[string]any{
							"url":    "https://github.com/myorg/checkout.git",
							"branch": "release/v2",
							"commit": "abc123",
						},
						"version": 5,
					}),
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			By("Reconciling the Workflow")
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Workflow status is updated")
			Eventually(func() bool {
				updatedWorkflow := &openchoreodevv1alpha1.Workflow{}
				if err := k8sClient.Get(ctx, typeNamespacedName, updatedWorkflow); err != nil {
					return false
				}
				return updatedWorkflow.Status.Phase == openchoreodevv1alpha1.WorkflowPhaseRunning
			}, timeout, interval).Should(BeTrue())

			By("Verifying the rendered Argo Workflow is created")
			var renderedWorkflow *unstructured.Unstructured
			Eventually(func() bool {
				workflowList := &unstructured.UnstructuredList{}
				workflowList.SetGroupVersionKind(argoWorkflowGVK())
				if err := k8sClient.List(ctx, workflowList); err != nil {
					return false
				}
				if len(workflowList.Items) > 0 {
					renderedWorkflow = &workflowList.Items[0]
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			By("Verifying the rendered workflow has correct metadata")
			Expect(renderedWorkflow.GetName()).To(ContainSubstring("checkout-service-"))
			Expect(renderedWorkflow.GetNamespace()).To(Equal("build-default"))
			labels := renderedWorkflow.GetLabels()
			Expect(labels["openchoreo.dev/component"]).To(Equal("checkout-service"))
			Expect(labels["openchoreo.dev/project"]).To(Equal("ecommerce"))

			By("Verifying the rendered workflow has correct spec")
			spec, found, err := unstructured.NestedMap(renderedWorkflow.Object, "spec")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())

			// Verify service account
			serviceAccount, _, _ := unstructured.NestedString(spec, "serviceAccountName")
			Expect(serviceAccount).To(Equal("build-bot"))

			// Verify arguments
			args, _, _ := unstructured.NestedMap(spec, "arguments")
			params, _, _ := unstructured.NestedSlice(args, "parameters")

			// Helper to find parameter value
			getParamValue := func(name string) string {
				for _, p := range params {
					param := p.(map[string]any)
					if param["name"] == name {
						return param["value"].(string)
					}
				}
				return ""
			}

			By("Verifying schema parameters are correctly rendered")
			Expect(getParamValue("repo-url")).To(Equal("https://github.com/myorg/checkout.git"))
			Expect(getParamValue("branch")).To(Equal("release/v2"))
			Expect(getParamValue("commit")).To(Equal("abc123"))

			// Version is int64, need to handle type assertion
			for _, p := range params {
				param := p.(map[string]any)
				if param["name"] == "version" {
					// Could be int64 or float64 depending on JSON unmarshaling
					switch v := param["value"].(type) {
					case int64:
						Expect(v).To(Equal(int64(5)))
					case float64:
						Expect(v).To(Equal(float64(5)))
					default:
						Fail("version parameter has unexpected type")
					}
				}
			}

			By("Verifying fixed parameters are correctly rendered")
			Expect(getParamValue("builder-image")).To(Equal("gcr.io/buildpacks/builder:v1"))

			By("Verifying ComponentTypeDefinition overrides are applied")
			Expect(getParamValue("security-scan")).To(Equal("false")) // Overridden from "true"
			Expect(getParamValue("timeout")).To(Equal("45m"))         // Overridden from "30m"

			By("Verifying workflowTemplateRef is preserved")
			templateRef, _, _ := unstructured.NestedMap(spec, "workflowTemplateRef")
			Expect(templateRef["name"]).To(Equal("buildpacks-template"))
		})

		It("should apply schema defaults when parameters are not provided", func() {
			By("Creating a Workflow with minimal parameters")
			workflow := &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowName + "-defaults",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					Owner: openchoreodevv1alpha1.WorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					WorkflowDefinitionRef: openchoreodevv1alpha1.WorkflowDefinitionReference{
						Name:      workflowDefName,
						Namespace: namespace,
					},
					Parameters: mustMarshalRaw(map[string]any{
						"repository": map[string]any{
							"url": "https://github.com/test/repo.git",
							// branch and commit not provided - should use defaults
						},
						// version not provided - should use default
					}),
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			By("Reconciling the Workflow")
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      workflowName + "-defaults",
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the rendered workflow has default values")
			Eventually(func() bool {
				workflowList := &unstructured.UnstructuredList{}
				workflowList.SetGroupVersionKind(argoWorkflowGVK())
				if err := k8sClient.List(ctx, workflowList); err != nil {
					return false
				}
				for _, item := range workflowList.Items {
					if item.GetName() != "" && item.GetLabels()["openchoreo.dev/component"] == "test-component" {
						spec, _, _ := unstructured.NestedMap(item.Object, "spec")
						args, _, _ := unstructured.NestedMap(spec, "arguments")
						params, _, _ := unstructured.NestedSlice(args, "parameters")

						for _, p := range params {
							param := p.(map[string]any)
							if param["name"] == "branch" && param["value"] == "main" {
								return true
							}
						}
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, workflow)).To(Succeed())
		})

		It("should handle missing WorkflowDefinition gracefully", func() {
			By("Creating a Workflow referencing non-existent WorkflowDefinition")
			workflow := &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowName + "-missing",
					Namespace: namespace,
				},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					Owner: openchoreodevv1alpha1.WorkflowOwner{
						ProjectName:   "test-project",
						ComponentName: "test-component",
					},
					WorkflowDefinitionRef: openchoreodevv1alpha1.WorkflowDefinitionReference{
						Name:      "non-existent-definition",
						Namespace: namespace,
					},
					Parameters: mustMarshalRaw(map[string]any{}),
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())

			By("Reconciling the Workflow")
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      workflowName + "-missing",
					Namespace: namespace,
				},
			})
			Expect(err).NotTo(HaveOccurred()) // Controller should handle gracefully

			By("Verifying the Workflow status shows error")
			Eventually(func() bool {
				updatedWorkflow := &openchoreodevv1alpha1.Workflow{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      workflowName + "-missing",
					Namespace: namespace,
				}, updatedWorkflow); err != nil {
					return false
				}
				return updatedWorkflow.Status.Phase == openchoreodevv1alpha1.WorkflowPhaseError
			}, timeout, interval).Should(BeTrue())

			// Cleanup
			Expect(k8sClient.Delete(ctx, workflow)).To(Succeed())
		})
	})
})

// Helper functions

func mustMarshalRaw(v any) *runtime.RawExtension {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return &runtime.RawExtension{Raw: data}
}

func argoWorkflowGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "WorkflowList",
	}
}
