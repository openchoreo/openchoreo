// Copyright 2026 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package k8sresources

import (
	"encoding/json"
	"strings"

	openchoreov1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	"github.com/openchoreo/openchoreo/internal/openchoreo-api/models"
)

// fluxInventoryResource coordinates parsed from Flux helm-controller inventory entry IDs.
type fluxInventoryResource struct {
	Namespace string
	Name      string
	Kind      string
	Group     string
	Version   string
}

var fluxInventoryKindAPI = map[string]fluxInventoryResource{
	"Deployment":              {Kind: "Deployment", Group: "apps", Version: "v1"},
	"ReplicaSet":              {Kind: "ReplicaSet", Group: "apps", Version: "v1"},
	"StatefulSet":             {Kind: "StatefulSet", Group: "apps", Version: "v1"},
	"DaemonSet":               {Kind: "DaemonSet", Group: "apps", Version: "v1"},
	"Job":                     {Kind: "Job", Group: "batch", Version: "v1"},
	"Pod":                     {Kind: "Pod", Version: "v1"},
	"PersistentVolumeClaim":   {Kind: "PersistentVolumeClaim", Version: "v1"},
}

// parseFluxHelmInventoryEntryID parses Flux helm-controller inventory entry IDs.
//
// Examples:
//
//	obt-dev_smollm2_apps_Deployment
//	obt-dev_smollm2-model-storage__PersistentVolumeClaim
func parseFluxHelmInventoryEntryID(id string) *fluxInventoryResource {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil
	}

	for kind, api := range fluxInventoryKindAPI {
		suffix := kind
		if kind == "PersistentVolumeClaim" {
			suffix = "__" + kind
		} else {
			suffix = "_" + kind
		}
		if !strings.HasSuffix(trimmed, suffix) {
			continue
		}
		prefix := trimmed[:len(trimmed)-len(suffix)]
		out := api

		if kind == "PersistentVolumeClaim" {
			splitAt := strings.Index(prefix, "_")
			if splitAt <= 0 {
				return nil
			}
			out.Namespace = prefix[:splitAt]
			out.Name = prefix[splitAt+1:]
			return &out
		}

		groupSplit := strings.LastIndex(prefix, "_")
		if groupSplit <= 0 {
			return nil
		}
		out.Group = prefix[groupSplit+1:]
		nsName := prefix[:groupSplit]
		nsSplit := strings.Index(nsName, "_")
		if nsSplit <= 0 {
			return nil
		}
		out.Namespace = nsName[:nsSplit]
		out.Name = nsName[nsSplit+1:]
		return &out
	}

	return nil
}

func inventoryEntryIDsFromStatus(status map[string]any) []string {
	inventory, _ := status["inventory"].(map[string]any)
	entries, _ := inventory["entries"].([]any)
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(getStringField(entryMap, "id"))
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func inventoryEntryIDsFromObject(obj map[string]any) []string {
	status, _ := obj["status"].(map[string]any)
	if status == nil {
		return nil
	}
	return inventoryEntryIDsFromStatus(status)
}

func inventoryEntryIDsFromManifestStatus(entry *openchoreov1alpha1.RenderedManifestStatus) []string {
	if entry == nil || entry.Status == nil || len(entry.Status.Raw) == 0 {
		return nil
	}
	var status map[string]any
	if err := json.Unmarshal(entry.Status.Raw, &status); err != nil {
		return nil
	}
	return inventoryEntryIDsFromStatus(status)
}

func helmManifestStatusHasInventoryKind(entry *openchoreov1alpha1.RenderedManifestStatus, kind string) bool {
	for _, id := range inventoryEntryIDsFromManifestStatus(entry) {
		parsed := parseFluxHelmInventoryEntryID(id)
		if parsed != nil && parsed.Kind == kind {
			return true
		}
	}
	return false
}

func helmInventoryResourceMatch(
	entry *openchoreov1alpha1.RenderedManifestStatus,
	group, version, kind, name string,
) (namespace string, matched bool) {
	for _, id := range inventoryEntryIDsFromManifestStatus(entry) {
		parsed := parseFluxHelmInventoryEntryID(id)
		if parsed == nil || parsed.Kind != kind || parsed.Name != name {
			continue
		}
		if parsed.Group != group {
			continue
		}
		if version != "" && parsed.Version != "" && parsed.Version != version {
			continue
		}
		return parsed.Namespace, true
	}
	return "", false
}

func dedupeResourceNodes(nodes []models.ResourceNode) []models.ResourceNode {
	seen := make(map[string]bool, len(nodes))
	out := make([]models.ResourceNode, 0, len(nodes))
	for _, node := range nodes {
		if node.UID == "" || seen[node.UID] {
			continue
		}
		seen[node.UID] = true
		out = append(out, node)
	}
	return out
}
