{{/*
The ClusterAuthzRoleBinding that grants the bundled Backstage portal's catalog
provider read access.

Only the binding lives here, not the role. The role stays in values.yaml under
openchoreoApi.config.security.authorization.bootstrap.roles and is seeded
whatever backstage.enabled is: it grants nothing until something is bound to it,
and a portal you run yourself needs the same actions. The binding is what
follows the portal - "openchoreo-control-plane.authz.allManifests" appends it
only when the portal is enabled, and
"openchoreo-control-plane.authz.removalManifests" deletes it when it is not.

It lives here rather than in bootstrap.mappings because the overlays that
override that list would otherwise each need a copy, and because its entitlement
has to be configurable for IdPs that identify the client through a different
claim (Dex encodes it into `sub`).

The entitlement follows the client the catalog provider actually presents (see
backstage/deployment.yaml OPENCHOREO_SERVICE_CLIENT_ID), so a deployment that
sets its own serviceClientId is bound correctly.
*/}}

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
