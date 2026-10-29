{{/*
Authz bootstrap objects that exist only for the bundled Backstage portal.

These live here, rather than in the bootstrap roles/mappings values, so that
`backstage.enabled=false` does not seed a role and a binding for a client that
was never registered with the IdP. They are appended by
"openchoreo-control-plane.authz.allManifests" and rendered through the same
roleManifest / bindingManifest helpers as every other bootstrap object.

The binding entitlement follows the client the catalog provider actually
presents (see backstage/deployment.yaml OPENCHOREO_SERVICE_CLIENT_ID), so a
deployment that sets its own serviceClientId is bound correctly.
*/}}

{{- define "openchoreo-control-plane.backstage.authzRole" -}}
name: backstage-catalog-reader
system: true
actions:
  - "component:view"
  - "releaseline:view"
  - "componenttype:view"
  - "resource:view"
  - "resourcerelease:view"
  - "projectrelease:view"
  - "resourcereleasebinding:view"
  - "projectreleasebinding:view"
  - "resourcetype:view"
  - "projecttype:view"
  - "namespace:view"
  - "project:view"
  - "dataplane:view"
  - "environment:view"
  - "trait:view"
  - "workload:view"
  - "workflowplane:view"
  - "clusterworkflowplane:view"
  - "workflow:view"
  - "deploymentpipeline:view"
  - "observabilityplane:view"
  - "clusterobservabilityplane:view"
  - "clusterdataplane:view"
  - "clustercomponenttype:view"
  - "clusterresourcetype:view"
  - "clusterprojecttype:view"
  - "clustertrait:view"
  - "clusterworkflow:view"
  - "observabilityalertsnotificationchannel:view"
{{- end }}

{{- define "openchoreo-control-plane.backstage.authzBinding" -}}
name: backstage-catalog-reader-binding
kind: ClusterAuthzRoleBinding
system: true
roleMappings:
  - roleRef:
      name: backstage-catalog-reader
      kind: ClusterAuthzRole
entitlement:
  claim: {{ .Values.backstage.auth.authzEntitlement.claim | default "client_id" | quote }}
  value: {{ .Values.backstage.auth.authzEntitlement.value | default .Values.backstage.auth.serviceClientId | default .Values.backstage.auth.clientId | quote }}
effect: allow
{{- end }}
