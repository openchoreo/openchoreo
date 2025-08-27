{{/*
Chart-specific aliases to openchoreo-common library functions
*/}}
{{- define "openchoreo-observability-plane.name" -}}
{{ include "openchoreo.name" . }}
{{- end }}

{{- define "openchoreo-observability-plane.fullname" -}}
{{ include "openchoreo.fullname" . }}
{{- end }}

{{- define "openchoreo-observability-plane.chart" -}}
{{ include "openchoreo.chart" . }}
{{- end }}

{{- define "openchoreo-observability-plane.labels" -}}
{{ include "openchoreo.labels" . }}
{{- end }}

{{- define "openchoreo-observability-plane.selectorLabels" -}}
{{ include "openchoreo.selectorLabels" . }}
{{- end }}

{{- define "openchoreo-observability-plane.serviceAccountName" -}}
{{ include "openchoreo.serviceAccountName" . }}
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
