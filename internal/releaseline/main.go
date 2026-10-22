// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package releaseline

import (
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
)

// MainLine is the keyword for a component's implicit main line. It has no ReleaseLine, and resources
// of the main line leave spec.owner.releaseLine empty.
const MainLine = "main"

// Normalize maps a release line given by a user to its stored form: the main line keyword and an
// empty name both become "".
func Normalize(line string) string {
	line = strings.TrimSpace(line)
	if line == MainLine {
		return ""
	}
	return line
}

// Environments summarizes a line's ReleaseBindings, one entry per environment, sorted by environment.
// An empty line selects the main line's bindings.
func Environments(bindings []openchoreov1alpha1.ReleaseBinding, projectName, componentName, line string) []openchoreov1alpha1.ReleaseLineEnvironment {
	var envs []openchoreov1alpha1.ReleaseLineEnvironment
	seen := map[string]bool{}
	for i := range bindings {
		rb := &bindings[i]
		if rb.Spec.Owner.ProjectName != projectName || rb.Spec.Owner.ComponentName != componentName ||
			rb.Spec.Owner.ReleaseLine != line || seen[rb.Spec.Environment] {
			continue
		}
		seen[rb.Spec.Environment] = true
		envs = append(envs, openchoreov1alpha1.ReleaseLineEnvironment{
			Environment:    rb.Spec.Environment,
			ReleaseBinding: rb.Name,
			Release:        rb.Spec.ReleaseName,
			Ready:          meta.IsStatusConditionTrue(rb.Status.Conditions, "Ready"),
		})
	}
	sort.Slice(envs, func(i, j int) bool { return envs[i].Environment < envs[j].Environment })
	return envs
}

// Main describes a component's implicit main line as a ReleaseLine, built from the component and its
// bindings without a line. It is never stored.
func Main(comp *openchoreov1alpha1.Component, bindings []openchoreov1alpha1.ReleaseBinding) *openchoreov1alpha1.ReleaseLine {
	return &openchoreov1alpha1.ReleaseLine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              MainLine,
			Namespace:         comp.Namespace,
			CreationTimestamp: comp.CreationTimestamp,
		},
		Spec: openchoreov1alpha1.ReleaseLineSpec{
			Owner: openchoreov1alpha1.ReleaseLineOwner{
				ProjectName:   comp.Spec.Owner.ProjectName,
				ComponentName: comp.Name,
			},
		},
		Status: openchoreov1alpha1.ReleaseLineStatus{
			ObservedGeneration: comp.Status.ObservedGeneration,
			Conditions:         comp.Status.Conditions,
			LatestRelease:      comp.Status.LatestRelease,
			Environments:       Environments(bindings, comp.Spec.Owner.ProjectName, comp.Name, ""),
		},
	}
}
