// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

var runID = fmt.Sprintf("%d", time.Now().UnixNano())

// testNS is the namespace used for the deployment pipeline deletion tests.
// It uses the default namespace since environments and dataplanes are expected there.
var testNS = "default"

// uniqueName returns a test-scoped unique name to avoid collisions between test runs.
func uniqueName(base string) string {
	// Use last 8 chars of runID for brevity
	suffix := runID
	if len(suffix) > 8 {
		suffix = suffix[len(suffix)-8:]
	}
	return fmt.Sprintf("%s-%s", base, suffix)
}

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

func deploymentPipelineYAML(name, envName string) string {
	dp := &openchoreov1alpha1.DeploymentPipeline{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "DeploymentPipeline",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.DeploymentPipelineSpec{
			PromotionPaths: []openchoreov1alpha1.PromotionPath{
				{
					SourceEnvironmentRef: openchoreov1alpha1.EnvironmentRef{
						Name: envName,
					},
					TargetEnvironmentRefs: []openchoreov1alpha1.TargetEnvironmentRef{},
				},
			},
		},
	}
	return mustYAMLDocs(dp)
}

func projectYAML(name, pipelineName string) string {
	p := &openchoreov1alpha1.Project{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openchoreo.dev/v1alpha1",
			Kind:       "Project",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: openchoreov1alpha1.ProjectSpec{
			DeploymentPipelineRef: &openchoreov1alpha1.DeploymentPipelineRef{
				Kind: openchoreov1alpha1.DeploymentPipelineRefKindDeploymentPipeline,
				Name: pipelineName,
			},
		},
	}
	return mustYAMLDocs(p)
}
