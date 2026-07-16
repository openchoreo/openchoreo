{{/*
Expand the name of the chart.
*/}}
{{- define "openchoreo-workflow-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "openchoreo-workflow-plane.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "openchoreo-workflow-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openchoreo-workflow-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openchoreo-workflow-plane.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Common labels
These labels should be applied to all resources and include:
- helm.sh/chart: Chart name and version
- app.kubernetes.io/name: Name of the application
- app.kubernetes.io/instance: Unique name identifying the instance of an application
- app.kubernetes.io/version: Current version of the application
- app.kubernetes.io/managed-by: Tool being used to manage the application
- app.kubernetes.io/part-of: Name of a higher level application this one is part of
*/}}
{{- define "openchoreo-workflow-plane.labels" -}}
helm.sh/chart: {{ include "openchoreo-workflow-plane.chart" . }}
{{ include "openchoreo-workflow-plane.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: openchoreo
{{- with .Values.global.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
These labels are used for pod selectors and should be stable across upgrades.
They should NOT include version or chart labels as these change with upgrades.
*/}}
{{- define "openchoreo-workflow-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo-workflow-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., argo-controller, workflow-template).

Usage:
  {{ include "openchoreo-workflow-plane.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-workflow-plane.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "argo-controller", "workflow-template", "rbac")
*/}}
{{- define "openchoreo-workflow-plane.componentLabels" -}}
{{ include "openchoreo-workflow-plane.labels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Component selector labels
Extends selector labels with component identification.
This should be used for:
  - spec.selector.matchLabels in Deployments, StatefulSets, DaemonSets
  - spec.selector in Services
  - metadata.labels in Pod templates

These labels must be stable and should not include version information.

Usage:
  {{ include "openchoreo-workflow-plane.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-workflow-plane.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "argo-controller", "workflow-template", "rbac")
*/}}
{{- define "openchoreo-workflow-plane.componentSelectorLabels" -}}
{{ include "openchoreo-workflow-plane.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Cluster Agent name
*/}}
{{- define "openchoreo-workflow-plane.clusterAgent.name" -}}
{{- default "cluster-agent" .Values.clusterAgent.name }}
{{- end }}

{{/*
Cluster Agent service account name
*/}}
{{- define "openchoreo-workflow-plane.clusterAgent.serviceAccountName" -}}
{{- if .Values.clusterAgent.serviceAccount.create }}
{{- default "cluster-agent" .Values.clusterAgent.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.clusterAgent.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Container image reference for a component.

Renders "<repository>:<tag>", with the tag defaulting to .Chart.AppVersion.
When global.imageRegistry is set, the registry host of the repository is
replaced with it so every first-party image resolves from a single private
or mirror registry. A leading path segment counts as a registry host only
if it contains "." or ":" or equals "localhost", the same rule Docker and
containerd use to parse image references. The override may itself carry a
path (e.g. "registry.example.com/ghcr.io") for path-preserving mirrors.

Note: images of the argo-workflows subchart are not covered by this value;
override them via argo-workflows.{controller,executor,server}.image.registry.

Usage:
  {{ include "openchoreo-workflow-plane.image" (dict "context" . "image" .Values.clusterAgent.image) }}

Parameters:
  - context: The current Helm context (usually .)
  - image: The component image block (repository, tag)
*/}}
{{- define "openchoreo-workflow-plane.image" -}}
{{- $repo := .image.repository -}}
{{- with .context.Values.global.imageRegistry -}}
{{- $parts := splitList "/" $repo -}}
{{- $first := first $parts -}}
{{- if and (gt (len $parts) 1) (or (contains "." $first) (contains ":" $first) (eq $first "localhost")) -}}
{{- $repo = join "/" (rest $parts) -}}
{{- end -}}
{{- $repo = printf "%s/%s" . $repo -}}
{{- end -}}
{{- printf "%s:%s" $repo (.image.tag | default .context.Chart.AppVersion) -}}
{{- end }}

