{{/*
Expand the name of the chart.
*/}}
{{- define "openchoreo-control-plane.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "openchoreo-control-plane.fullname" -}}
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
{{- define "openchoreo-control-plane.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "openchoreo-control-plane.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "openchoreo-control-plane.fullname" .) .Values.serviceAccount.name }}
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
{{- define "openchoreo-control-plane.labels" -}}
helm.sh/chart: {{ include "openchoreo-control-plane.chart" . }}
{{ include "openchoreo-control-plane.selectorLabels" . }}
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
{{- define "openchoreo-control-plane.selectorLabels" -}}
app.kubernetes.io/name: {{ include "openchoreo-control-plane.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., controller-manager, api-server).

Usage:
  {{ include "openchoreo-control-plane.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-control-plane.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "api-server", "controller", "worker")
*/}}
{{- define "openchoreo-control-plane.componentLabels" -}}
{{ include "openchoreo-control-plane.labels" .context }}
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
  {{ include "openchoreo-control-plane.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-control-plane.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "api-server", "controller", "worker")
*/}}
{{- define "openchoreo-control-plane.componentSelectorLabels" -}}
{{ include "openchoreo-control-plane.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Backstage secrets name.
Uses existingSecret when provided, otherwise falls back to the chart-generated secret.
*/}}
{{- define "openchoreo-control-plane.backstage.secretName" -}}
{{- .Values.backstage.existingSecret | default (printf "%s-backstage-secrets" (include "openchoreo-control-plane.fullname" .)) }}
{{- end }}

{{/*
Backstage service account name
Always returns openchoreo-backstage (or custom name from values).
Service account is always created when backstage is enabled for security.
*/}}
{{- define "openchoreo-control-plane.backstage.serviceAccountName" -}}
{{- default "openchoreo-backstage" .Values.backstage.serviceAccount.name }}
{{- end }}

{{/*
OpenChoreo API service account name
*/}}
{{- define "openchoreo-control-plane.openchoreoApi.serviceAccountName" -}}
{{- default "openchoreo-api" .Values.openchoreoApi.serviceAccount.name }}
{{- end }}

{{/*
OpenChoreo API resource name
Returns a static name for openchoreo-api resources (Service, Deployment, ClusterRole, etc.)
This keeps resource names clean and consistent (e.g., "openchoreo-api" instead of "my-release-openchoreo-control-plane-api")
*/}}
{{- define "openchoreo-control-plane.openchoreoApi.name" -}}
{{- default "openchoreo-api" .Values.openchoreoApi.name }}
{{- end }}

{{/*
Cluster Gateway resource name
*/}}
{{- define "openchoreo-control-plane.clusterGateway.name" -}}
{{- default "cluster-gateway" .Values.clusterGateway.name }}
{{- end }}

{{/*
Cluster Gateway service account name
*/}}
{{- define "openchoreo-control-plane.clusterGateway.serviceAccountName" -}}
{{- if .Values.clusterGateway.serviceAccount.create }}
{{- default "cluster-gateway" .Values.clusterGateway.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.clusterGateway.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Get the API server hostname
Precedence: ingress.hosts > openchoreoApi.config.server.publicUrl > baseDomain derivation
*/}}
{{- define "openchoreo.apiHost" -}}
{{- if .Values.openchoreoApi.ingress.hosts -}}
  {{- (index .Values.openchoreoApi.ingress.hosts 0).host -}}
{{- else if .Values.openchoreoApi.config.server.publicUrl -}}
  {{- $url := .Values.openchoreoApi.config.server.publicUrl | trimPrefix "http://" | trimPrefix "https://" -}}
  {{- $url = $url | splitList "/" | first -}}
  {{- $url | splitList ":" | first -}}
{{- else if .Values.global.baseDomain -}}
  {{- printf "api.%s" .Values.global.baseDomain -}}
{{- else -}}
  {{- fail "Set one of openchoreoApi.ingress.hosts, openchoreoApi.config.server.publicUrl, or global.baseDomain" -}}
{{- end -}}
{{- end -}}

{{/*
Get the Console (Backstage) hostname
*/}}
{{- define "openchoreo.consoleHost" -}}
{{- if .Values.backstage.ingress.hosts -}}
  {{- (index .Values.backstage.ingress.hosts 0).host -}}
{{- else if .Values.backstage.baseUrl -}}
  {{- $url := .Values.backstage.baseUrl | trimPrefix "http://" | trimPrefix "https://" -}}
  {{- $url = $url | splitList "/" | first -}}
  {{- $url | splitList ":" | first -}}
{{- else if .Values.global.baseDomain -}}
  {{- .Values.global.baseDomain -}}
{{- else -}}
  {{- fail "Set one of backstage.ingress.hosts, backstage.baseUrl, or global.baseDomain" -}}
{{- end -}}
{{- end -}}

{{/*
Get the Thunder IDP hostname
*/}}
{{- define "openchoreo.thunderHost" -}}
{{- if .Values.thunder.configuration.server.publicUrl -}}
  {{- $url := .Values.thunder.configuration.server.publicUrl -}}
  {{- $url = $url | trimPrefix "http://" | trimPrefix "https://" -}}
  {{- $url | splitList ":" | first -}}
{{- else if .Values.global.baseDomain -}}
  {{- printf "thunder.%s" .Values.global.baseDomain -}}
{{- else -}}
  {{- fail "Either global.baseDomain or thunder.configuration.server.publicUrl must be set" -}}
{{- end -}}
{{- end -}}

{{/*
Get the Console (Backstage) external base URL
*/}}
{{- define "openchoreo.consoleExternalUrl" -}}
{{- if .Values.backstage.baseUrl -}}
{{- .Values.backstage.baseUrl | trimSuffix "/" -}}
{{- else -}}
{{- printf "%s://%s%s" (include "openchoreo.protocol" .) (include "openchoreo.consoleHost" .) (include "openchoreo.port" .) -}}
{{- end -}}
{{- end -}}

{{/*
Check if TLS is enabled
*/}}
{{- define "openchoreo.tlsEnabled" -}}
{{- if .Values.global.tls.enabled -}}
true
{{- else -}}
false
{{- end -}}
{{- end -}}

{{/*
Get the protocol (http or https)
*/}}
{{- define "openchoreo.protocol" -}}
{{- if .Values.global.tls.enabled -}}
https
{{- else -}}
http
{{- end -}}
{{- end -}}

{{/*
Get the port suffix for URLs (e.g., ":8080" for non-standard ports)
*/}}
{{- define "openchoreo.port" -}}
{{- .Values.global.port | default "" -}}
{{- end -}}

{{/*
Get the external port number (for Thunder config)
- Custom port: strip colon from global.port (e.g., ":8080" -> "8080")
- TLS: 443
- no TLS: 80
*/}}
{{- define "openchoreo.externalPort" -}}
{{- if .Values.global.port -}}
{{- .Values.global.port | trimPrefix ":" -}}
{{- else if .Values.global.tls.enabled -}}
443
{{- else -}}
80
{{- end -}}
{{- end -}}

{{/*
Get Thunder internal URL for pod-to-pod communication
*/}}
{{- define "openchoreo.thunderInternalUrl" -}}
{{- printf "http://%s-service.%s.svc.cluster.local:%d" .Values.thunder.fullnameOverride .Release.Namespace (.Values.thunder.service.port | int) -}}
{{- end -}}

{{/*
Get the API server external URL for external communication
*/}}
{{- define "openchoreo.apiExternalUrl" -}}
{{- if .Values.openchoreoApi.config.server.publicUrl -}}
{{- .Values.openchoreoApi.config.server.publicUrl | trimSuffix "/" -}}
{{- else -}}
{{- printf "%s://%s%s" (include "openchoreo.protocol" .) (include "openchoreo.apiHost" .) (include "openchoreo.port" .) -}}
{{- end -}}
{{- end -}}

{{/*
Get the Thunder external URL for external communication (e.g., OIDC issuer)
*/}}
{{- define "openchoreo.thunderExternalUrl" -}}
{{- if .Values.thunder.configuration.server.publicUrl -}}
{{- .Values.thunder.configuration.server.publicUrl | trimSuffix "/" -}}
{{- else -}}
{{- printf "%s://%s%s" (include "openchoreo.protocol" .) (include "openchoreo.thunderHost" .) (include "openchoreo.port" .) -}}
{{- end -}}
{{- end -}}

{{/*
Get the scheme (http or https) - alias for protocol
*/}}
{{- define "openchoreo.scheme" -}}
{{- include "openchoreo.protocol" . -}}
{{- end -}}

{{/*
Check if HTTP-only mode (no TLS)
*/}}
{{- define "openchoreo.httpOnly" -}}
{{- if .Values.global.tls.enabled -}}
false
{{- else -}}
true
{{- end -}}
{{- end -}}
