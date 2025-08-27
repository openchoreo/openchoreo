{{/*
OpenChoreo Common Library Chart - Shared Helper Templates

This library chart provides shared helper templates for all OpenChoreo charts.
Charts should include this as a dependency and use these shared functions.

Usage:
  In Chart.yaml:
    dependencies:
      - name: openchoreo-common
        version: "^0.0.0"
        repository: "file://../openchoreo-common"

  In templates:
    {{ include "openchoreo.name" . }}
    {{ include "openchoreo.labels" . }}
    {{ include "openchoreo.componentLabels" (dict "context" . "component" "my-component") }}
*/}}

{{/*
Expand the name of the chart.
Takes the chart name from .Chart.Name, with optional override from .Values.nameOverride.
Truncates at 63 chars to comply with Kubernetes DNS naming spec.
*/}}
{{- define "openchoreo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name to avoid duplication.
*/}}
{{- define "openchoreo.fullname" -}}
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
{{- define "openchoreo.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
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
{{- define "openchoreo.labels" -}}
helm.sh/chart: {{ include "openchoreo.chart" . }}
{{ include "openchoreo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: openchoreo
{{- if and .Values.global .Values.global.commonLabels }}
{{ toYaml .Values.global.commonLabels }}
{{- end }}
{{- end }}

{{/*
Selector labels
These labels are used for pod selectors and should be stable across upgrades.
They should NOT include version or chart labels as these change with upgrades.
*/}}
{{- define "openchoreo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application.

Usage:
  {{ include "openchoreo.componentLabels" (dict "context" . "component" "my-component") }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "api-server", "controller", "worker")
*/}}
{{- define "openchoreo.componentLabels" -}}
{{ include "openchoreo.labels" .context }}
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
  {{ include "openchoreo.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "api-server", "controller", "worker")
*/}}
{{- define "openchoreo.componentSelectorLabels" -}}
{{ include "openchoreo.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openchoreo.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openchoreo.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}