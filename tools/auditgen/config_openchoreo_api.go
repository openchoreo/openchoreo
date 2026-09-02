// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/openchoreo/openchoreo/tools/internal/auditgen"

// apiExcludedOperationIDs are state-modifying routes deliberately not turned
// into an OperationDef here — see internal/openchoreo-api/audit/exemptions.go
// for the reason each is exempt rather than audited.
//
// GenerateRelease is NOT here: ComponentService.GenerateRelease calls
// s.k8sClient.Create on a new ComponentRelease
// (internal/openchoreo-api/services/component/service.go), so it needs a
// real definition — apiGenerateReleaseOverride below — rather than an
// exemption.
var apiExcludedOperationIDs = map[string]bool{
	"Evaluates":       true,
	"HandleAutoBuild": true,
}

// apiGenerateReleaseOverride replaces deriveDefinition's generic verb+resourceType
// derivation for GenerateRelease: the path's own resource segment ("components")
// names the parent the release is generated *from*, not the resource actually
// created (a new ComponentRelease), so the generic rule would both misclassify
// ResourceType and wrongly treat {componentName} as the created resource's own
// identifying path parameter.
var apiGenerateReleaseOverride = auditgen.OperationDef{
	ID: "GenerateRelease", Action: "generate_release", ResourceType: "componentrelease",
	Category: "CategoryManagement",
}

// apiActionSuffixSegments are trailing path segments that name a verb on an
// otherwise ordinary resource path, not a further level of resource
// hierarchy — e.g. ".../releasebindings/{releaseBindingName}/trigger". They
// are skipped when locating both the resource's own path parameter and its
// resource-kind segment.
var apiActionSuffixSegments = map[string]bool{
	"trigger":          true,
	"generate-release": true,
}

// apiResourceCategories maps every resource kind (the plural REST path segment)
// that a state-modifying operation can target to its audit Category. This is
// exhaustive, not a default-plus-exceptions table: deriveDefinition fails
// generation for any kind segment missing here, so adding a new resource
// kind to the API forces a deliberate category choice instead of silently
// landing in CategoryManagement. Add the new kind's entry here as part of
// that change.
var apiResourceCategories = map[string]string{
	"authzroles":               "CategoryAuthorization",
	"authzrolebindings":        "CategoryAuthorization",
	"clusterauthzroles":        "CategoryAuthorization",
	"clusterauthzrolebindings": "CategoryAuthorization",

	"clustercomponenttypes":                   "CategoryManagement",
	"clusterdataplanes":                       "CategoryManagement",
	"clusterobservabilityplanes":              "CategoryManagement",
	"clusterprojecttypes":                     "CategoryManagement",
	"clusterresourcetypes":                    "CategoryManagement",
	"clustertraits":                           "CategoryManagement",
	"clusterworkflows":                        "CategoryManagement",
	"clusterworkflowplanes":                   "CategoryManagement",
	"components":                              "CategoryManagement",
	"componentreleases":                       "CategoryManagement",
	"componenttypes":                          "CategoryManagement",
	"dataplanes":                              "CategoryManagement",
	"deploymentpipelines":                     "CategoryManagement",
	"environments":                            "CategoryManagement",
	"gitsecrets":                              "CategoryManagement",
	"namespaces":                              "CategoryManagement",
	"observabilityalertsnotificationchannels": "CategoryManagement",
	"observabilityplanes":                     "CategoryManagement",
	"projects":                                "CategoryManagement",
	"projectreleases":                         "CategoryManagement",
	"projectreleasebindings":                  "CategoryManagement",
	"projecttypes":                            "CategoryManagement",
	"releasebindings":                         "CategoryManagement",
	"resources":                               "CategoryManagement",
	"resourcereleases":                        "CategoryManagement",
	"resourcereleasebindings":                 "CategoryManagement",
	"resourcetypes":                           "CategoryManagement",
	"secrets":                                 "CategoryManagement",
	"secretreferences":                        "CategoryManagement",
	"traits":                                  "CategoryManagement",
	"workflows":                               "CategoryManagement",
	"workflowplanes":                          "CategoryManagement",
	"workflowruns":                            "CategoryManagement",
	"workloads":                               "CategoryManagement",
}

// apiSingularOverrides names resource-kind segments whose singular form isn't a
// bare trailing-"s" strip. Empty today — every kind currently in the spec
// singularizes mechanically — but kept as an explicit map rather than adding
// general inflector logic to singularize for cases that don't exist yet.
var apiSingularOverrides = map[string]string{}

// apiConfig returns openchoreo-api's own auditgen.Config.
func apiConfig() auditgen.Config {
	return auditgen.Config{
		ResourceCategories:   apiResourceCategories,
		ExcludedOperationIDs: apiExcludedOperationIDs,
		SingularOverrides:    apiSingularOverrides,
		ActionSuffixSegments: apiActionSuffixSegments,
		Overrides: map[string]auditgen.OperationDef{
			apiGenerateReleaseOverride.ID: apiGenerateReleaseOverride,
		},
	}
}
