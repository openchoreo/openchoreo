{{/*
Expand the name of the chart.
*/}}
{{- define "openchoreo-observability-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "openchoreo-observability-plane.fullname" -}}
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
{{- define "openchoreo-observability-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openchoreo-observability-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openchoreo-observability-plane.fullname" .) .Values.serviceAccount.name }}
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
{{- define "openchoreo-observability-plane.labels" -}}
helm.sh/chart: {{ include "openchoreo-observability-plane.chart" . }}
{{ include "openchoreo-observability-plane.selectorLabels" . }}
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
{{- define "openchoreo-observability-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo-observability-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., opensearch, dashboard, observer).

Usage:
  {{ include "openchoreo-observability-plane.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-observability-plane.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "opensearch", "dashboard", "observer")
*/}}
{{- define "openchoreo-observability-plane.componentLabels" -}}
{{ include "openchoreo-observability-plane.labels" .context }}
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
  {{ include "openchoreo-observability-plane.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-observability-plane.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "opensearch", "dashboard", "observer")
*/}}
{{- define "openchoreo-observability-plane.componentSelectorLabels" -}}
{{ include "openchoreo-observability-plane.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Cluster Agent name
*/}}
{{- define "openchoreo-observability-plane.clusterAgent.name" -}}
{{- default "cluster-agent" .Values.clusterAgent.name }}
{{- end }}

{{/*
Cluster Agent service account name
*/}}
{{- define "openchoreo-observability-plane.clusterAgent.serviceAccountName" -}}
{{- if .Values.clusterAgent.serviceAccount.create }}
{{- default "cluster-agent" .Values.clusterAgent.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.clusterAgent.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Validate that placeholder .invalid hostnames have been replaced with real domains.
The chart ships .invalid defaults for cross-cluster URLs (the observability plane
typically runs on a separate cluster from the control plane), so they must be set
explicitly per deployment. k3d overlays supply real values.
*/}}
{{- define "openchoreo-observability-plane.validateHostnames" -}}
{{- $errors := list -}}
{{- if contains ".invalid" .Values.observer.controlPlaneApiUrl -}}
  {{- $errors = append $errors "observer.controlPlaneApiUrl contains placeholder domain (.invalid)" -}}
{{- end -}}
{{- if contains ".invalid" (toYaml .Values.observer.extraEnvs) -}}
  {{- $errors = append $errors "observer.extraEnvs contains placeholder domain (.invalid) (e.g. OBSERVER_BASE_URL)" -}}
{{- end -}}
{{- if .Values.rca.enabled -}}
  {{- if contains ".invalid" .Values.rca.openchoreoApiUrl -}}
    {{- $errors = append $errors "rca.openchoreoApiUrl contains placeholder domain (.invalid)" -}}
  {{- end -}}
{{- end -}}
{{- if .Values.finOpsAgent.enabled -}}
  {{- if contains ".invalid" .Values.finOpsAgent.openchoreoApiUrl -}}
    {{- $errors = append $errors "finOpsAgent.openchoreoApiUrl contains placeholder domain (.invalid)" -}}
  {{- end -}}
{{- end -}}
{{- if gt (len $errors) 0 -}}
  {{- fail (printf "Placeholder domains found. Set real URLs for:\n  - %s" (join "\n  - " $errors)) -}}
{{- end -}}
{{- end -}}
