// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package workflowrun

//var _ = Describe("Workflow Controller", func() {
//	const (
//		namespace = "default"
//		timeout   = time.Second * 30
//		interval  = time.Millisecond * 250
//	)
//
//	var (
//		ctx                  context.Context
//		workflowDef          *openchoreodevv1alpha1.WorkflowDefinition
//		componentTypeDef     *openchoreodevv1alpha1.ComponentTypeDefinition
//		component            *openchoreodevv1alpha1.Component
//		buildPlane           *openchoreodevv1alpha1.BuildPlane
//		reconciler           *Reconciler
//		workflowDefName      string
//		componentTypeDefName string
//		componentName        string
//		buildPlaneName       string
//	)
//
//	BeforeEach(func() {
//		ctx = context.Background()
//
//		// Generate unique names for this test
//		workflowDefName = fmt.Sprintf("test-workflow-def-%d", time.Now().UnixNano())
//		componentTypeDefName = fmt.Sprintf("test-component-type-%d", time.Now().UnixNano())
//		componentName = fmt.Sprintf("test-component-%d", time.Now().UnixNano())
//		buildPlaneName = fmt.Sprintf("test-build-plane-%d", time.Now().UnixNano())
//
//		// Create BuildPlane with mock cluster spec
//		buildPlane = &openchoreodevv1alpha1.BuildPlane{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      buildPlaneName,
//				Namespace: namespace,
//				Labels: map[string]string{
//					labels.LabelKeyOrganizationName: namespace,
//				},
//			},
//			Spec: openchoreodevv1alpha1.BuildPlaneSpec{
//				KubernetesCluster: openchoreodevv1alpha1.KubernetesClusterSpec{
//					Server: "https://mock-build-plane:6443",
//					Auth: openchoreodevv1alpha1.KubernetesAuth{
//						BearerToken: &openchoreodevv1alpha1.ValueFrom{
//							Value: "mock-token",
//						},
//					},
//				},
//			},
//		}
//		Expect(k8sClient.Create(ctx, buildPlane)).To(Succeed())
//
//		// Create WorkflowDefinition with schema and template
//		workflowDef = &openchoreodevv1alpha1.WorkflowDefinition{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      workflowDefName,
//				Namespace: namespace,
//			},
//			Spec: openchoreodevv1alpha1.WorkflowDefinitionSpec{
//				Schema: mustMarshalRaw(map[string]any{
//					"repository": map[string]any{
//						"url":    "string",
//						"branch": "string | default=main",
//						"commit": "string | default=HEAD",
//					},
//					"version": "integer | default=1",
//				}),
//				FixedParameters: []openchoreodevv1alpha1.WorkflowParameter{
//					{Name: "builder_image", Value: "gcr.io/buildpacks/builder:v1"},
//					{Name: "security_scan", Value: "true"},
//					{Name: "timeout", Value: "30m"},
//				},
//				Resource: openchoreodevv1alpha1.WorkflowResource{
//					Template: mustMarshalRaw(map[string]any{
//						"apiVersion": "argoproj.io/v1alpha1",
//						"kind":       "Workflow",
//						"metadata": map[string]any{
//							"name":      "${ctx.componentName}-${ctx.uuid}",
//							"namespace": "build-${ctx.orgName}",
//							"labels": map[string]any{
//								"openchoreo.dev/component": "${ctx.componentName}",
//								"openchoreo.dev/project":   "${ctx.projectName}",
//							},
//						},
//						"spec": map[string]any{
//							"serviceAccountName": "build-bot",
//							"arguments": map[string]any{
//								"parameters": []any{
//									map[string]any{"name": "repo-url", "value": "${schema.repository.url}"},
//									map[string]any{"name": "branch", "value": "${schema.repository.branch}"},
//									map[string]any{"name": "commit", "value": "${schema.repository.commit}"},
//									map[string]any{"name": "version", "value": "${schema.version}"},
//									map[string]any{"name": "builder-image", "value": "${fixedParameters.builder_image}"},
//									map[string]any{"name": "security-scan", "value": "${fixedParameters.security_scan}"},
//									map[string]any{"name": "timeout", "value": "${fixedParameters.timeout}"},
//								},
//							},
//							"workflowTemplateRef": map[string]any{
//								"name": "buildpacks-template",
//							},
//						},
//					}),
//				},
//			},
//		}
//		Expect(k8sClient.Create(ctx, workflowDef)).To(Succeed())
//
//		// Create ComponentTypeDefinition with parameter overrides
//		componentTypeDef = &openchoreodevv1alpha1.ComponentTypeDefinition{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      componentTypeDefName,
//				Namespace: namespace,
//			},
//			Spec: openchoreodevv1alpha1.ComponentTypeDefinitionSpec{
//				WorkloadType: "deployment",
//				Resources: []openchoreodevv1alpha1.ResourceTemplate{
//					{
//						ID:       "deployment",
//						Template: mustMarshalRaw(map[string]any{"apiVersion": "apps/v1", "kind": "Deployment"}),
//					},
//				},
//				Build: &openchoreodevv1alpha1.ComponentTypeBuildConfig{
//					AllowedTemplates: []openchoreodevv1alpha1.AllowedWorkflowTemplate{
//						{
//							Name: workflowDefName,
//							FixedParameters: []openchoreodevv1alpha1.WorkflowParameter{
//								{Name: "security_scan", Value: "false"}, // Override
//								{Name: "timeout", Value: "45m"},         // Override
//							},
//						},
//					},
//				},
//			},
//		}
//		Expect(k8sClient.Create(ctx, componentTypeDef)).To(Succeed())
//
//		// Create Component
//		component = &openchoreodevv1alpha1.Component{
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      componentName,
//				Namespace: namespace,
//				Labels: map[string]string{
//					labels.LabelKeyOrganizationName: namespace,
//					labels.LabelKeyProjectName:      "test-project",
//					labels.LabelKeyName:             componentName,
//				},
//			},
//			Spec: openchoreodevv1alpha1.ComponentSpec{
//				Owner: openchoreodevv1alpha1.ComponentOwner{
//					ProjectName: "test-project",
//				},
//				ComponentType: fmt.Sprintf("deployment/%s", componentTypeDefName),
//				Build: openchoreodevv1alpha1.BuildSpecInComponent{
//					WorkflowTemplate: workflowDefName,
//				},
//			},
//		}
//		Expect(k8sClient.Create(ctx, component)).To(Succeed())
//
//		// Create reconciler with client manager
//		// The BuildPlane uses mock credentials and tests won't make real external API calls
//		reconciler = &Reconciler{
//			Client:       k8sClient,
//			Scheme:       k8sClient.Scheme(),
//			k8sClientMgr: kubernetesClient.NewManager(),
//		}
//	})
//
//	AfterEach(func() {
//		// Clean up in reverse order of creation
//		if component != nil {
//			_ = k8sClient.Delete(ctx, component)
//		}
//		if componentTypeDef != nil {
//			_ = k8sClient.Delete(ctx, componentTypeDef)
//		}
//		if workflowDef != nil {
//			_ = k8sClient.Delete(ctx, workflowDef)
//		}
//		if buildPlane != nil {
//			_ = k8sClient.Delete(ctx, buildPlane)
//		}
//
//		// Clean up any Argo Workflows
//		argoWorkflowList := &unstructured.UnstructuredList{}
//		argoWorkflowList.SetGroupVersionKind(argoWorkflowListGVK())
//		_ = k8sClient.List(ctx, argoWorkflowList)
//		for _, item := range argoWorkflowList.Items {
//			_ = k8sClient.Delete(ctx, &item)
//		}
//
//		// Clean up any Workloads
//		workloadList := &openchoreodevv1alpha1.WorkloadList{}
//		_ = k8sClient.List(ctx, workloadList, client.InNamespace(namespace))
//		for _, workload := range workloadList.Items {
//			_ = k8sClient.Delete(ctx, &workload)
//		}
//	})
//
//	Context("When creating a Workflow", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//		})
//
//		It("should set WorkflowPending condition on initial reconcile", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/myorg/checkout.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			_, err := reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			Expect(err).NotTo(HaveOccurred())
//
//			Eventually(func() bool {
//				updatedWorkflow := &openchoreodevv1alpha1.Workflow{}
//				if err := k8sClient.Get(ctx, types.NamespacedName{Name: workflowName, Namespace: namespace}, updatedWorkflow); err != nil {
//					return false
//				}
//				return isWorkflowInitiated(updatedWorkflow)
//			}, timeout, interval).Should(BeTrue())
//		})
//
//		It("should apply schema defaults when parameters are not provided", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//							// branch and commit not provided - should use defaults
//						},
//						// version not provided - should use default
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile twice
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//
//		It("should handle missing Component gracefully", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: "non-existent-component",
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema:                mustMarshalRaw(map[string]any{}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile twice
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, err := reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			// Should not error but requeue
//			Expect(err).NotTo(HaveOccurred())
//		})
//
//		It("should handle missing WorkflowDefinition gracefully", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: "non-existent-def",
//					Schema:                mustMarshalRaw(map[string]any{}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile twice
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, err := reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			// Should not error but requeue
//			Expect(err).NotTo(HaveOccurred())
//		})
//	})
//
//	Context("When syncing Argo Workflow status", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//		var argoWorkflow *unstructured.Unstructured
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//
//			// Create workflow
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile to create Argo Workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//			if argoWorkflow != nil {
//				_ = k8sClient.Delete(ctx, argoWorkflow)
//			}
//		})
//	})
//
//	Context("When extracting workload from Argo Workflow", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//		var argoWorkflow *unstructured.Unstructured
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//
//			// Create workflow
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile to create Argo Workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//			if argoWorkflow != nil {
//				_ = k8sClient.Delete(ctx, argoWorkflow)
//			}
//		})
//
//	})
//
//	Context("When workflow is deleted", func() {
//		It("should not error on reconcile", func() {
//			workflowName := fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//
//			result, err := reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			Expect(err).NotTo(HaveOccurred())
//			Expect(result).To(Equal(ctrl.Result{}))
//		})
//	})
//
//	Context("When workflow conditions are managed", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//		})
//
//		It("should set WorkflowRunning condition when Argo Workflow is running", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Initial reconcile to set pending
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//
//			// Verify WorkflowPending condition is set
//			Eventually(func() bool {
//				updatedWorkflow := &openchoreodevv1alpha1.Workflow{}
//				if err := k8sClient.Get(ctx, types.NamespacedName{Name: workflowName, Namespace: namespace}, updatedWorkflow); err != nil {
//					return false
//				}
//				return isWorkflowInitiated(updatedWorkflow) && !isWorkflowCompleted(updatedWorkflow)
//			}, timeout, interval).Should(BeTrue())
//		})
//	})
//
//	Context("When handling workload extraction failures", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//		var argoWorkflow *unstructured.Unstructured
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Create workflow and wait for Argo Workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//			if argoWorkflow != nil {
//				_ = k8sClient.Delete(ctx, argoWorkflow)
//			}
//		})
//	})
//
//	Context("When rendering with CEL context variables", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//		})
//
//		It("should correctly evaluate all CEL context variables", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile to render workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//	})
//
//	Context("When handling cross-namespace resources", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//		})
//
//		It("should add tracking labels for cross-namespace resources", func() {
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Reconcile to render workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//	})
//
//	Context("When handling status timestamps", func() {
//		var workflow *openchoreodevv1alpha1.Workflow
//		var workflowName string
//		var argoWorkflow *unstructured.Unstructured
//
//		BeforeEach(func() {
//			workflowName = fmt.Sprintf("test-workflow-%d", time.Now().UnixNano())
//
//			workflow = &openchoreodevv1alpha1.Workflow{
//				ObjectMeta: metav1.ObjectMeta{
//					Name:      workflowName,
//					Namespace: namespace,
//					Labels: map[string]string{
//						labels.LabelKeyOrganizationName: namespace,
//						labels.LabelKeyProjectName:      "test-project",
//						labels.LabelKeyComponentName:    componentName,
//					},
//				},
//				Spec: openchoreodevv1alpha1.WorkflowSpec{
//					Owner: openchoreodevv1alpha1.WorkflowOwner{
//						ProjectName:   "test-project",
//						ComponentName: componentName,
//					},
//					WorkflowDefinitionRef: workflowDefName,
//					Schema: mustMarshalRaw(map[string]any{
//						"repository": map[string]any{
//							"url": "https://github.com/test/repo.git",
//						},
//					}),
//				},
//			}
//			Expect(k8sClient.Create(ctx, workflow)).To(Succeed())
//
//			// Create workflow and wait for Argo Workflow
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//			_, _ = reconciler.Reconcile(ctx, reconcile.Request{
//				NamespacedName: types.NamespacedName{Name: workflowName, Namespace: namespace},
//			})
//		})
//
//		AfterEach(func() {
//			if workflow != nil {
//				_ = k8sClient.Delete(ctx, workflow)
//			}
//			if argoWorkflow != nil {
//				_ = k8sClient.Delete(ctx, argoWorkflow)
//			}
//		})
//	})
//
//	Context("Helper functions", func() {
//		It("should parse ComponentTypeDefinition name correctly", func() {
//			name, err := parseComponentTypeDefinitionName("deployment/my-component-type")
//			Expect(err).NotTo(HaveOccurred())
//			Expect(name).To(Equal("my-component-type"))
//		})
//
//		It("should return error for invalid ComponentType format", func() {
//			_, err := parseComponentTypeDefinitionName("invalid-format")
//			Expect(err).To(HaveOccurred())
//		})
//
//		It("should convert parameters to strings correctly", func() {
//			resource := map[string]any{
//				"apiVersion": "argoproj.io/v1alpha1",
//				"kind":       "Workflow",
//				"spec": map[string]any{
//					"arguments": map[string]any{
//						"parameters": []any{
//							map[string]any{"name": "int-param", "value": 42},
//							map[string]any{"name": "bool-param", "value": true},
//							map[string]any{"name": "string-param", "value": "hello"},
//							map[string]any{"name": "array-param", "value": []any{1, 2, 3}},
//						},
//					},
//				},
//			}
//
//			converted := convertParameterValuesToStrings(resource)
//			spec := converted["spec"].(map[string]any)
//			args := spec["arguments"].(map[string]any)
//			params := args["parameters"].([]any)
//
//			for _, p := range params {
//				param := p.(map[string]any)
//				value := param["value"]
//				Expect(value).To(BeAssignableToTypeOf(""))
//			}
//		})
//
//		It("should extract workload CR from Argo Workflow correctly", func() {
//			workloadYAML := "apiVersion: openchoreo.dev/v1alpha1\nkind: Workload"
//
//			argoWorkflow := &argoproj.Workflow{
//				Status: argoproj.WorkflowStatus{
//					Nodes: map[string]argoproj.NodeStatus{
//						"node-1": {
//							TemplateName: "workload-create-step",
//							Phase:        argoproj.NodeSucceeded,
//							Outputs: &argoproj.Outputs{
//								Parameters: []argoproj.Parameter{
//									{
//										Name:  "workload-cr",
//										Value: ptr.To(argoproj.AnyString(workloadYAML)),
//									},
//								},
//							},
//						},
//					},
//				},
//			}
//
//			result := extractWorkloadCRFromArgoWorkflow(argoWorkflow)
//			Expect(result).To(Equal(workloadYAML))
//		})
//
//		It("should return empty string when workload step not found", func() {
//			argoWorkflow := &argoproj.Workflow{
//				Status: argoproj.WorkflowStatus{
//					Nodes: map[string]argoproj.NodeStatus{
//						"node-1": {
//							TemplateName: "other-step",
//							Phase:        argoproj.NodeSucceeded,
//						},
//					},
//				},
//			}
//
//			result := extractWorkloadCRFromArgoWorkflow(argoWorkflow)
//			Expect(result).To(Equal(""))
//		})
//	})
//})
//
//// Helper functions
//
//func mustMarshalRaw(v any) *runtime.RawExtension {
//	data, err := json.Marshal(v)
//	if err != nil {
//		panic(err)
//	}
//	return &runtime.RawExtension{Raw: data}
//}
//
//func argoWorkflowListGVK() schema.GroupVersionKind {
//	return schema.GroupVersionKind{
//		Group:   "argoproj.io",
//		Version: "v1alpha1",
//		Kind:    "WorkflowList",
//	}
//}
