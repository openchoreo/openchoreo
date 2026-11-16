{{/*
Authz bootstrap helpers.

These render the default authorization roles and bindings that seed the
control plane. They are consumed by the post-install/post-upgrade bootstrap
hook (see bootstrap-configmap.yaml / bootstrap-job.yaml) rather than applied
as tracked release resources, since a hook Job is always waited on to
completion (retried via its own backoffLimit) independent of whether the
caller passes --wait to helm.
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
Full multi-document YAML for every bootstrap role and binding, at column 0.
Roles before bindings. Consumed by the bootstrap ConfigMap.
*/}}
{{- define "openchoreo-control-plane.authz.allManifests" -}}
{{- $root := . -}}
{{- $bootstrap := .Values.openchoreoApi.config.security.authorization.bootstrap -}}
{{- range $r := $bootstrap.roles }}
---
{{ include "openchoreo-control-plane.authz.roleManifest" (dict "role" $r "root" $root) }}
{{- end }}
{{- range $m := $bootstrap.mappings }}
---
{{ include "openchoreo-control-plane.authz.bindingManifest" (dict "mapping" $m "root" $root) }}
{{- end }}
{{- end }}
