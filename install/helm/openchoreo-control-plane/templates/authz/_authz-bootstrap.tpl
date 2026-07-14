{{/*
Authz bootstrap helpers.

These render the default authorization roles and bindings that seed the
control plane. They are consumed by the post-install/post-upgrade bootstrap
hook (see bootstrap-configmap.yaml / bootstrap-job.yaml) rather than applied
as tracked release resources, since a hook Job is always waited on to
completion (retried via its own backoffLimit) independent of whether the
caller passes --wait to helm.

All objects, system (hardcoded below, labeled openchoreo.io/system: "true")
and user-facing (sourced from values), are upserted via kubectl apply on
every hook run, so they always match the chart version currently installed.
Hand-edits made directly against these objects in the cluster do not survive
the next install/upgrade; customize via values instead.
*/}}

{{/*
Bootstrap resource name prefix (cluster-scoped and namespaced hook resources).
*/}}
{{- define "openchoreo-control-plane.authz.bootstrapName" -}}
{{- printf "%s-authz-bootstrap" (include "openchoreo-control-plane.fullname" .) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Render a single AuthzRole/ClusterAuthzRole document.
Usage: include "openchoreo-control-plane.authz.roleManifest" (dict "role" $role "root" $root)
*/}}
{{- define "openchoreo-control-plane.authz.roleManifest" -}}
{{- $role := .role -}}
{{- $root := .root -}}
{{- if eq (default "" $role.namespace) "" }}
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRole
metadata:
  name: {{ $role.name }}
  labels:
    {{- include "openchoreo-control-plane.labels" $root | nindent 4 }}
    openchoreo.io/bootstrap: "true"
    {{- if $role.system }}
    openchoreo.io/system: "true"
    {{- end }}
spec:
  {{- if and $role.description (ne $role.description "") }}
  description: {{ $role.description | quote }}
  {{- end }}
  actions:
    {{- range $role.actions }}
    - {{ . | quote }}
    {{- end }}
{{- else }}
apiVersion: openchoreo.dev/v1alpha1
kind: AuthzRole
metadata:
  name: {{ $role.name }}
  namespace: {{ $role.namespace }}
  labels:
    {{- include "openchoreo-control-plane.labels" $root | nindent 4 }}
    openchoreo.io/bootstrap: "true"
    {{- if $role.system }}
    openchoreo.io/system: "true"
    {{- end }}
spec:
  {{- if and $role.description (ne $role.description "") }}
  description: {{ $role.description | quote }}
  {{- end }}
  actions:
    {{- range $role.actions }}
    - {{ . | quote }}
    {{- end }}
{{- end }}
{{- end }}

{{/*
Render a single AuthzRoleBinding/ClusterAuthzRoleBinding document.
Usage: include "openchoreo-control-plane.authz.bindingManifest" (dict "mapping" $m "root" $root)
*/}}
{{- define "openchoreo-control-plane.authz.bindingManifest" -}}
{{- $m := .mapping -}}
{{- $root := .root -}}
{{- $bindingKind := default "ClusterAuthzRoleBinding" $m.kind -}}
{{- if eq $bindingKind "ClusterAuthzRoleBinding" }}
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: {{ $m.name }}
  labels:
    {{- include "openchoreo-control-plane.labels" $root | nindent 4 }}
    openchoreo.io/bootstrap: "true"
    {{- if $m.system }}
    openchoreo.io/system: "true"
    {{- end }}
spec:
  roleMappings:
    {{- range $m.roleMappings }}
    - roleRef:
        name: {{ .roleRef.name }}
        kind: {{ .roleRef.kind }}
      {{- if and .scope (or .scope.namespace .scope.project .scope.component) }}
      scope:
        {{- if .scope.namespace }}
        namespace: {{ .scope.namespace }}
        {{- end }}
        {{- if .scope.project }}
        project: {{ .scope.project }}
        {{- end }}
        {{- if .scope.component }}
        component: {{ .scope.component }}
        {{- end }}
      {{- end }}
    {{- end }}
  entitlement:
    claim: {{ $m.entitlement.claim | quote }}
    value: {{ $m.entitlement.value | quote }}
  effect: {{ $m.effect | quote }}
{{- else }}
apiVersion: openchoreo.dev/v1alpha1
kind: AuthzRoleBinding
metadata:
  name: {{ $m.name }}
  namespace: {{ $m.namespace }}
  labels:
    {{- include "openchoreo-control-plane.labels" $root | nindent 4 }}
    openchoreo.io/bootstrap: "true"
    {{- if $m.system }}
    openchoreo.io/system: "true"
    {{- end }}
spec:
  roleMappings:
    {{- range $m.roleMappings }}
    - roleRef:
        name: {{ .roleRef.name }}
        kind: {{ .roleRef.kind }}
      {{- if and .scope (or .scope.project .scope.component) }}
      scope:
        {{- if .scope.project }}
        project: {{ .scope.project }}
        {{- end }}
        {{- if .scope.component }}
        component: {{ .scope.component }}
        {{- end }}
      {{- end }}
    {{- end }}
  entitlement:
    claim: {{ $m.entitlement.claim | quote }}
    value: {{ $m.entitlement.value | quote }}
  effect: {{ $m.effect | quote }}
{{- end }}
{{- end }}

{{/*
Hardcoded system roles. These are platform-owned and always managed by the
bootstrap hook (upserted), so they are intentionally not exposed in values.
*/}}
{{- define "openchoreo-control-plane.authz.systemRoles" -}}
- name: backstage-catalog-reader
  system: true
  actions:
    - "component:view"
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
- name: finops-agent
  system: true
  actions:
    - "component:view"
    - "project:view"
    - "namespace:view"
    - "environment:view"
    - "metrics:view"
    - "alerts:view"
- name: rca-agent
  system: true
  actions:
    - "component:view"
    - "project:view"
    - "namespace:view"
    - "componentrelease:view"
    - "releasebinding:view"
    - "workflowrun:view"
    - "environment:view"
    - "workload:view"
    - "trait:view"
    - "logs:view"
    - "events:view"
    - "metrics:view"
    - "alerts:view"
    - "incidents:view"
    - "incidents:update"
    - "traces:view"
- name: workload-publisher
  system: true
  actions:
    - "workload:create"
    - "workload:update"
    - "workload:view"
    - "workflowrun:view"
    - "workflowrun:update"
- name: observer-resource-reader
  system: true
  actions:
    - "component:view"
    - "project:view"
    - "namespace:view"
    - "environment:view"
{{- end }}

{{/*
Hardcoded system bindings (see systemRoles).
*/}}
{{- define "openchoreo-control-plane.authz.systemMappings" -}}
- name: backstage-catalog-reader-binding
  kind: ClusterAuthzRoleBinding
  system: true
  roleMappings:
    - roleRef:
        name: backstage-catalog-reader
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: openchoreo-backstage-client
  effect: allow
- name: finops-agent-binding
  kind: ClusterAuthzRoleBinding
  system: true
  roleMappings:
    - roleRef:
        name: finops-agent
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: openchoreo-finops-agent
  effect: allow
- name: rca-agent-binding
  kind: ClusterAuthzRoleBinding
  system: true
  roleMappings:
    - roleRef:
        name: rca-agent
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: openchoreo-rca-agent
  effect: allow
- name: workload-publisher-binding
  kind: ClusterAuthzRoleBinding
  system: true
  roleMappings:
    - roleRef:
        name: workload-publisher
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: openchoreo-workload-publisher-client
  effect: allow
- name: observer-resource-reader-binding
  kind: ClusterAuthzRoleBinding
  system: true
  roleMappings:
    - roleRef:
        name: observer-resource-reader
        kind: ClusterAuthzRole
  entitlement:
    claim: sub
    value: openchoreo-observer-resource-reader-client
  effect: allow
{{- end }}

{{/*
Full multi-document YAML for every bootstrap role and binding, system
(hardcoded) and user-facing (from values), at column 0. All are applied
uniformly, roles before bindings. Consumed by the bootstrap ConfigMap.
*/}}
{{- define "openchoreo-control-plane.authz.allManifests" -}}
{{- $root := . -}}
{{- $bootstrap := .Values.openchoreoApi.config.security.authorization.bootstrap -}}
{{- range $r := (include "openchoreo-control-plane.authz.systemRoles" . | fromYamlArray) }}
---
{{ include "openchoreo-control-plane.authz.roleManifest" (dict "role" $r "root" $root) }}
{{- end }}
{{- range $r := $bootstrap.roles }}
{{- if not $r.system }}
---
{{ include "openchoreo-control-plane.authz.roleManifest" (dict "role" $r "root" $root) }}
{{- end }}
{{- end }}
{{- range $m := (include "openchoreo-control-plane.authz.systemMappings" . | fromYamlArray) }}
---
{{ include "openchoreo-control-plane.authz.bindingManifest" (dict "mapping" $m "root" $root) }}
{{- end }}
{{- range $m := $bootstrap.mappings }}
{{- if not $m.system }}
---
{{ include "openchoreo-control-plane.authz.bindingManifest" (dict "mapping" $m "root" $root) }}
{{- end }}
{{- end }}
{{- end }}
