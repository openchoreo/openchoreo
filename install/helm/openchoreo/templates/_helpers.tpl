{{/*
Expand the name of the chart.
*/}}
{{- define "openchoreo.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
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
*/}}
{{- define "openchoreo.labels" -}}
helm.sh/chart: {{ include "openchoreo.chart" . }}
{{ include "openchoreo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "openchoreo.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Controller Manager service account name
*/}}
{{- define "openchoreo.controllerManager.serviceAccountName" -}}
{{- if .Values.controllerManager.serviceAccount.create }}
{{- default (include "openchoreo.fullname" . | printf "%s-operator") .Values.controllerManager.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.controllerManager.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Backstage service account name
*/}}
{{- define "openchoreo.backstage.serviceAccountName" -}}
{{- if .Values.backstage.serviceAccount.create }}
{{- default (include "openchoreo.fullname" . | printf "%s-ui") .Values.backstage.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.backstage.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
OpenChoreo API service account name
*/}}
{{- define "openchoreo.openchoreoApi.serviceAccountName" -}}
{{- if .Values.openchoreoApi.serviceAccount.create }}
{{- default (include "openchoreo.fullname" . | printf "%s-api") .Values.openchoreoApi.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.openchoreoApi.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Control plane labels (for compatibility with default resources)
*/}}
{{- define "openchoreo-control-plane.labels" -}}
helm.sh/chart: {{ include "openchoreo.chart" . }}
{{ include "openchoreo.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: openchoreo
{{- with .Values.global.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}
