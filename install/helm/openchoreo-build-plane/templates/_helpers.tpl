{{/*
Expand the name of the chart.
*/}}
{{- define "openchoreo-build-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "openchoreo-build-plane.fullname" -}}
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
{{- define "openchoreo-build-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openchoreo-build-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openchoreo-build-plane.fullname" .) .Values.serviceAccount.name }}
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
{{- define "openchoreo-build-plane.labels" -}}
helm.sh/chart: {{ include "openchoreo-build-plane.chart" . }}
{{ include "openchoreo-build-plane.selectorLabels" . }}
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
{{- define "openchoreo-build-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo-build-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., argo-controller, workflow-template).

Usage:
  {{ include "openchoreo-build-plane.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-build-plane.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "argo-controller", "workflow-template", "rbac")
*/}}
{{- define "openchoreo-build-plane.componentLabels" -}}
{{ include "openchoreo-build-plane.labels" .context }}
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
  {{ include "openchoreo-build-plane.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-build-plane.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "argo-controller", "workflow-template", "rbac")
*/}}
{{- define "openchoreo-build-plane.componentSelectorLabels" -}}
{{ include "openchoreo-build-plane.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}
