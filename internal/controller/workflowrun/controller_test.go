// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	openchoreodevv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var _ = Describe("WorkflowRun Controller", func() {
	Context("When reconciling a WorkflowRun resource", func() {
		const resourceName = "test-workflowrun"
		const workflowName = "test-workflow"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("Creating the WorkflowRun resource")
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: workflowName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, workflowRun)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the WorkflowRun resource")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			if err := k8sClient.Get(ctx, typeNamespacedName, resource); err == nil {
				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
					controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully create the resource", func() {
			By("Verifying the WorkflowRun was created")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Name).To(Equal(resourceName))
			Expect(resource.Spec.Workflow.Name).To(Equal(workflowName))
		})

		It("should handle reconciliation when resource not found", func() {
			By("Reconciling a non-existent resource")
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			nonExistentName := types.NamespacedName{
				Name:      "non-existent",
				Namespace: "default",
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nonExistentName,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
		})

		It("should have empty status initially", func() {
			By("Verifying the WorkflowRun has empty status")
			resource := &openchoreodevv1alpha1.WorkflowRun{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(resource.Status.Conditions).To(BeEmpty())
			Expect(resource.Status.RunReference).To(BeNil())
			Expect(resource.Status.Resources).To(BeNil())
		})
	})

	Context("When working with status fields", func() {
		It("should correctly set and retrieve RunReference", func() {
			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-run-ref",
					Namespace: "default",
				},
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					RunReference: &openchoreodevv1alpha1.ResourceReference{
						APIVersion: "argoproj.io/v1alpha1",
						Kind:       "Workflow",
						Name:       "test-workflow-run",
						Namespace:  "build-namespace",
					},
				},
			}

			Expect(cwf.Status.RunReference).NotTo(BeNil())
			Expect(cwf.Status.RunReference.APIVersion).To(Equal("argoproj.io/v1alpha1"))
			Expect(cwf.Status.RunReference.Kind).To(Equal("Workflow"))
			Expect(cwf.Status.RunReference.Name).To(Equal("test-workflow-run"))
			Expect(cwf.Status.RunReference.Namespace).To(Equal("build-namespace"))
		})

		It("should correctly set and retrieve Resources", func() {
			resources := []openchoreodevv1alpha1.ResourceReference{
				{
					APIVersion: "v1",
					Kind:       "Secret",
					Name:       "registry-credentials",
					Namespace:  "build-namespace",
				},
				{
					APIVersion: "v1",
					Kind:       "ConfigMap",
					Name:       "build-config",
					Namespace:  "build-namespace",
				},
			}

			cwf := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-resources",
					Namespace: "default",
				},
				Status: openchoreodevv1alpha1.WorkflowRunStatus{
					Resources: &resources,
				},
			}

			Expect(cwf.Status.Resources).NotTo(BeNil())
			Expect(*cwf.Status.Resources).To(HaveLen(2))
			Expect((*cwf.Status.Resources)[0].Kind).To(Equal("Secret"))
			Expect((*cwf.Status.Resources)[0].Name).To(Equal("registry-credentials"))
			Expect((*cwf.Status.Resources)[1].Kind).To(Equal("ConfigMap"))
			Expect((*cwf.Status.Resources)[1].Name).To(Equal("build-config"))
		})
	})
})

// Unit tests for helper functions
var _ = Describe("Helper Functions", func() {
	Describe("extractServiceAccountName", func() {
		It("should extract service account name from resource", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "my-service-account",
				},
			}

			result, err := extractServiceAccountName(resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("my-service-account"))
		})

		It("should return error when spec not found", func() {
			resource := map[string]any{
				"metadata": map[string]any{},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec not found"))
		})

		It("should return error when serviceAccountName not found", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"otherField": "value",
				},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("serviceAccountName not found"))
		})

		It("should return error when serviceAccountName is empty", func() {
			resource := map[string]any{
				"spec": map[string]any{
					"serviceAccountName": "",
				},
			}

			_, err := extractServiceAccountName(resource)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("extractRunResourceNamespace", func() {
		It("should extract namespace from resource", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"namespace": "my-namespace",
					"name":      "my-resource",
				},
			}

			result, err := extractRunResourceNamespace(resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("my-namespace"))
		})

		It("should return error when metadata not found", func() {
			resource := map[string]any{
				"spec": map[string]any{},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("metadata not found"))
		})

		It("should return error when namespace not found", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"name": "my-resource",
				},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("namespace not found"))
		})

		It("should return error when namespace is empty", func() {
			resource := map[string]any{
				"metadata": map[string]any{
					"namespace": "",
				},
			}

			_, err := extractRunResourceNamespace(resource)
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("convertToString", func() {
		It("should convert string to string", func() {
			result := convertToString("hello")
			Expect(result).To(Equal("hello"))
		})

		It("should convert int to string", func() {
			result := convertToString(42)
			Expect(result).To(Equal("42"))
		})

		It("should convert int32 to string", func() {
			result := convertToString(int32(42))
			Expect(result).To(Equal("42"))
		})

		It("should convert int64 to string", func() {
			result := convertToString(int64(42))
			Expect(result).To(Equal("42"))
		})

		It("should convert float32 to string", func() {
			result := convertToString(float32(3.14))
			Expect(result).To(ContainSubstring("3.14"))
		})

		It("should convert float64 to string", func() {
			result := convertToString(3.14159)
			Expect(result).To(ContainSubstring("3.14"))
		})

		It("should convert bool to string", func() {
			Expect(convertToString(true)).To(Equal("true"))
			Expect(convertToString(false)).To(Equal("false"))
		})

		It("should convert map to JSON string", func() {
			input := map[string]any{
				"key1": "value1",
				"key2": 42,
			}
			result := convertToString(input)
			var decoded map[string]any
			err := json.Unmarshal([]byte(result), &decoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded["key1"]).To(Equal("value1"))
		})

		It("should convert slice to JSON string", func() {
			input := []any{"item1", "item2", 3}
			result := convertToString(input)
			var decoded []any
			err := json.Unmarshal([]byte(result), &decoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(decoded).To(HaveLen(3))
		})
	})

	Describe("convertParameterValuesToStrings", func() {
		It("should convert parameter values in workflow resource", func() {
			resource := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"spec": map[string]any{
					"arguments": map[string]any{
						"parameters": []any{
							map[string]any{
								"name":  "param1",
								"value": 42,
							},
							map[string]any{
								"name":  "param2",
								"value": true,
							},
						},
					},
				},
			}

			result := convertParameterValuesToStrings(resource)
			spec := result["spec"].(map[string]any)
			args := spec["arguments"].(map[string]any)
			params := args["parameters"].([]any)

			param1 := params[0].(map[string]any)
			Expect(param1["value"]).To(Equal("42"))

			param2 := params[1].(map[string]any)
			Expect(param2["value"]).To(Equal("true"))
		})

		It("should preserve non-parameter fields", func() {
			resource := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name": "test",
				},
			}

			result := convertParameterValuesToStrings(resource)
			Expect(result["apiVersion"]).To(Equal("argoproj.io/v1alpha1"))
			Expect(result["kind"]).To(Equal("Workflow"))
		})
	})

	Context("When testing TTL inheritance for WorkflowRun", func() {
		const workflowWithTTL = "workflow-with-ttl-inherit"

		ctx := context.Background()

		BeforeEach(func() {
			By("Creating a Workflow with TTLAfterCompletion set")
			// Create a minimal valid workflow template for testing
			runTemplate := map[string]any{
				"apiVersion": "argoproj.io/v1alpha1",
				"kind":       "Workflow",
				"metadata": map[string]any{
					"name":      "${metadata.workflowRunName}",
					"namespace": "${metadata.namespaceName}",
				},
				"spec": map[string]any{
					"entrypoint": "main",
					"templates": []any{
						map[string]any{
							"name": "main",
							"container": map[string]any{
								"image":   "alpine:latest",
								"command": []string{"echo", "hello"},
							},
						},
					},
				},
			}

			runTemplateJSON, err := json.Marshal(runTemplate)
			Expect(err).NotTo(HaveOccurred())

			workflow := &openchoreodevv1alpha1.Workflow{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workflowWithTTL,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowSpec{
					RunTemplate:        &runtime.RawExtension{Raw: runTemplateJSON},
					TTLAfterCompletion: "1h",
				},
			}
			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the Workflow resource")
			workflow := &openchoreodevv1alpha1.Workflow{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      workflowWithTTL,
				Namespace: "default",
			}, workflow); err == nil {
				Expect(k8sClient.Delete(ctx, workflow)).To(Succeed())
			}
		})

		It("should have empty TTL when WorkflowRun is created without TTLAfterCompletion", func() {
			By("Creating a WorkflowRun without TTL")
			resourceName := "test-ttl-inheritance"
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: workflowWithTTL,
					},
					// No TTLAfterCompletion set - TTL inheritance tested in controller logic
				},
			}
			Expect(k8sClient.Create(ctx, workflowRun)).To(Succeed())

			defer func() {
				By("Cleaning up the WorkflowRun resource")
				resource := &openchoreodevv1alpha1.WorkflowRun{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				}, resource); err == nil {
					if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
						controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
						Expect(k8sClient.Update(ctx, resource)).To(Succeed())
					}
					Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				}
			}()

			By("Verifying TTL is initially empty and references the parent Workflow")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				}, workflowRun)
			}, "5s", "500ms").Should(Succeed())

			Expect(workflowRun.Spec.TTLAfterCompletion).To(Equal(""))
			Expect(workflowRun.Spec.Workflow.Name).To(Equal(workflowWithTTL))

			By("Verifying the parent Workflow has TTLAfterCompletion set")
			workflow := &openchoreodevv1alpha1.Workflow{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      workflowWithTTL,
				Namespace: "default",
			}, workflow)).To(Succeed())
			Expect(workflow.Spec.TTLAfterCompletion).To(Equal("1h"))
		})

		It("should not override explicit TTL with Workflow TTL", func() {
			By("Creating a WorkflowRun with explicit TTL")
			resourceName := "test-ttl-override"
			explicitTTL := "30m"
			workflowRun := &openchoreodevv1alpha1.WorkflowRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: openchoreodevv1alpha1.WorkflowRunSpec{
					Workflow: openchoreodevv1alpha1.WorkflowRunConfig{
						Name: workflowWithTTL,
					},
					TTLAfterCompletion: explicitTTL, // Explicit TTL should not be overridden
				},
			}
			Expect(k8sClient.Create(ctx, workflowRun)).To(Succeed())

			defer func() {
				By("Cleaning up the WorkflowRun resource")
				resource := &openchoreodevv1alpha1.WorkflowRun{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				}, resource); err == nil {
					if controllerutil.ContainsFinalizer(resource, WorkflowRunCleanupFinalizer) {
						controllerutil.RemoveFinalizer(resource, WorkflowRunCleanupFinalizer)
						Expect(k8sClient.Update(ctx, resource)).To(Succeed())
					}
					Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
				}
			}()

			By("Reconciling the WorkflowRun")
			reconciler := &Reconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying TTL was not overridden")
			Consistently(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resourceName,
					Namespace: "default",
				}, workflowRun)
				if err != nil {
					return ""
				}
				return workflowRun.Spec.TTLAfterCompletion
			}, "2s", "500ms").Should(Equal(explicitTTL))
		})
	})
})
