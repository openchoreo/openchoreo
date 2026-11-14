// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package lsp_test

const (
	testValueAProj                           = "/a/proj"
	testValueChangeNameToDockerGitopsRelease = "Change name to \"docker-gitops-release\""
	webApplicationTypeYAML                   = "apiVersion: openchoreo.dev/v1alpha1\nkind: ComponentType\nmetadata:\n  name: web-application\nspec:\n  allowedTraits:\n    - name: persistent-volume\n"
	persistentVolumeTraitYAML                = "apiVersion: openchoreo.dev/v1alpha1\nkind: Trait\nmetadata:\n  name: persistent-volume\n"
	releaseWorkflowYAML                      = "apiVersion: openchoreo.dev/v1alpha1\nkind: Workflow\nmetadata:\n  name: docker-gitops-release\n  namespace: default\n"
	testValueFileVirtualOpenYaml             = "file:///virtual/open.yaml"
	testValueName                            = "name"
)
