// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

const (
	clusterDataPlane = "e2e-shared"
	openChoreoAPIVer = "openchoreo.dev/v1alpha1"
)

func mustYAMLDocs(objects ...any) string {
	docs := make([]string, 0, len(objects))
	for _, obj := range objects {
		data, err := yaml.Marshal(obj)
		if err != nil {
			panic(fmt.Sprintf("failed to marshal yaml document: %v", err))
		}
		docs = append(docs, strings.TrimSpace(string(data)))
	}
	return strings.Join(docs, "\n---\n")
}

// cpNamespaceYAML renders a control-plane namespace (labelled so the
// openchoreo-api recognises it as a CP namespace). Used by the authorization
// cases that need additional namespaces beyond the suite's primary mcpNs.
func cpNamespaceYAML(ns string) string {
	return fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    openchoreo.dev/control-plane: "true"
`, ns)
}

func platformResourcesYAML(cpNamespace string, environments []string, projects []string) string {
	promotionPaths := make([]openchoreov1alpha1.PromotionPath, 0)

	if len(environments) == 0 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: "development"},
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name: "development",
			}},
		})
	} else if len(environments) == 1 {
		promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
			SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: environments[0]},
			TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
				Name: environments[0],
			}},
		})
	} else {
		for i := 0; i < len(environments)-1; i++ {
			promotionPaths = append(promotionPaths, openchoreov1alpha1.PromotionPath{
				SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{Name: environments[i]},
				TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{{
					Name: environments[i+1],
				}},
			})
		}
	}

	docs := []any{
		&openchoreov1alpha1.DeploymentPipeline{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "DeploymentPipeline"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "default",
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": "default",
				},
			},
			Spec: openchoreov1alpha1.DeploymentPipelineSpec{PromotionPaths: promotionPaths},
		},
	}

	for _, env := range environments {
		docs = append(docs, &openchoreov1alpha1.Environment{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Environment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      env,
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": env,
				},
			},
			Spec: openchoreov1alpha1.EnvironmentSpec{
				DataPlaneRef: &openchoreov1alpha1.DataPlaneRef{
					Kind: openchoreov1alpha1.DataPlaneRefKindClusterDataPlane,
					Name: clusterDataPlane,
				},
				IsProduction: false,
			},
		})
	}

	for _, proj := range projects {
		docs = append(docs, &openchoreov1alpha1.Project{
			TypeMeta: metav1.TypeMeta{APIVersion: openChoreoAPIVer, Kind: "Project"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      proj,
				Namespace: cpNamespace,
				Labels: map[string]string{
					"openchoreo.dev/name": proj,
				},
			},
			Spec: openchoreov1alpha1.ProjectSpec{DeploymentPipelineRef: openchoreov1alpha1.DeploymentPipelineRef{Name: "default"}},
		})
	}

	return mustYAMLDocs(docs...)
}
