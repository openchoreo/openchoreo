{{/*
Expand the name of the chart.
*/}}
{{- define "security-scanner.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "security-scanner.fullname" -}}
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
{{- define "security-scanner.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "security-scanner.labels" -}}
helm.sh/chart: {{ include "security-scanner.chart" . }}
{{ include "security-scanner.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "security-scanner.selectorLabels" -}}
app.kubernetes.io/name: {{ include "security-scanner.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service account name
*/}}
{{- define "security-scanner.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "security-scanner.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Validate SQLite configuration requirements
*/}}
{{- define "security-scanner.validateSQLite" -}}
{{- if eq .Values.database.backend "sqlite" }}
  {{- if not (eq (int .Values.replicas) 1) }}
    {{- fail "SQLite backend requires replicas to be set to 1 to prevent database corruption" }}
  {{- end }}
  {{- if not .Values.persistence.enabled }}
    {{- fail "SQLite backend requires persistence to be enabled" }}
  {{- end }}
{{- end }}
{{- end }}
