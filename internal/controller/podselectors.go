// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/openchoreo/openchoreo/internal/labels"
)

// PodSelectorInput identifies one line of a component in one environment.
type PodSelectorInput struct {
	NamespaceName   string
	ProjectName     string
	ComponentName   string
	EnvironmentName string
	ComponentUID    string
	EnvironmentUID  string
	ProjectUID      string

	// ReleaseLine and ReleaseLineUID name the component's release line, both
	// empty for its main line.
	ReleaseLine    string
	ReleaseLineUID string
}

// ComponentPodSelectors returns the labels that select the pods of a
// component's line in an environment. The ReleaseBinding controller renders
// them onto the pods; anything selecting those pods from elsewhere, such as an
// Endpoint's network policy, must compute them here so the two cannot drift.
func ComponentPodSelectors(in PodSelectorInput) map[string]string {
	selectors := map[string]string{
		labels.LabelKeyNamespaceName:   in.NamespaceName,
		labels.LabelKeyProjectName:     in.ProjectName,
		labels.LabelKeyComponentName:   in.ComponentName,
		labels.LabelKeyEnvironmentName: in.EnvironmentName,
		labels.LabelKeyComponentUID:    in.ComponentUID,
		labels.LabelKeyEnvironmentUID:  in.EnvironmentUID,
		labels.LabelKeyProjectUID:      in.ProjectUID,
	}
	if in.ReleaseLine != "" {
		selectors[labels.LabelKeyReleaseLine] = in.ReleaseLine
		selectors[labels.LabelKeyReleaseLineUID] = in.ReleaseLineUID
		// The main line selects on the component UID, so a line's pods carry it under another key.
		delete(selectors, labels.LabelKeyComponentUID)
		selectors[labels.LabelKeyReleaseLineComponentUID] = in.ComponentUID
	}
	return selectors
}
