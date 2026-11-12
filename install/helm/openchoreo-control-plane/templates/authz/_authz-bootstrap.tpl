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
{{/*
The binding that points the bundled portal's client at backstage-catalog-reader
is appended here rather than declared in bootstrap.mappings, because the
overlays that replace that list wholesale would otherwise each need a copy.

A deployment that still declares the binding itself wins: the job applies this
stream in order, so appending ours would overwrite theirs by name. That matters
because a deployment on an external IdP binds a different claim (Dex identifies
the client through `sub`), and replacing it with the default `client_id` binding
breaks catalog sync with a 403 and nothing else would say why.
*/}}
{{- if and $root.Values.backstage.enabled (not (include "openchoreo-control-plane.authz.portalBindingDeclared" $root)) }}
---
{{ include "openchoreo-control-plane.authz.bindingManifest" (dict "mapping" (include "openchoreo-control-plane.backstage.authzBinding" $root | fromYaml) "root" $root) }}
{{- end }}
{{- end }}

{{/*
Whether bootstrap.mappings already declares the portal's binding by name, in
which case the chart neither appends its own nor removes the deployment's.
Renders a non-empty string for yes, nothing for no.
*/}}
{{- define "openchoreo-control-plane.authz.portalBindingDeclared" -}}
{{- $root := . -}}
{{- $name := (include "openchoreo-control-plane.backstage.authzBinding" $root | fromYaml).name -}}
{{- range $m := $root.Values.openchoreoApi.config.security.authorization.bootstrap.mappings }}
{{- if eq (default "" $m.name) $name }}declared{{ end }}
{{- end }}
{{- end }}

{{/*
Objects an earlier revision of this release seeded that these values no longer
do, as a multi-document stream of just apiVersion/kind/name. The bootstrap job
deletes these before applying the rest, because `kubectl apply` cannot remove
what is no longer in its input (see bootstrap-job.yaml).

Only the bundled portal's binding qualifies. Turning the portal off has to drop
it: it grants the portal's OAuth client cluster-wide read, and the quick-start
installs that client with a published default secret. The role it names stays
seeded either way, and every other bootstrap object is seeded unconditionally,
so nothing else can be left behind.
*/}}
{{- define "openchoreo-control-plane.authz.removalManifests" -}}
{{- $root := . -}}
{{- if and (not $root.Values.backstage.enabled) (not (include "openchoreo-control-plane.authz.portalBindingDeclared" $root)) }}
{{- $portalBinding := include "openchoreo-control-plane.backstage.authzBinding" $root | fromYaml }}
---
apiVersion: openchoreo.dev/v1alpha1
kind: ClusterAuthzRoleBinding
metadata:
  name: {{ $portalBinding.name }}
{{- end }}
{{- end }}
