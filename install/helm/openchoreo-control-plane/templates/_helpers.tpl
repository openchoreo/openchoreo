{{/*
Chart-specific aliases to openchoreo-common library functions
*/}}
{{- define "openchoreo-control-plane.name" -}}
{{ include "openchoreo.name" . }}
{{- end }}

{{- define "openchoreo-control-plane.fullname" -}}
{{ include "openchoreo.fullname" . }}
{{- end }}

{{- define "openchoreo-control-plane.chart" -}}
{{ include "openchoreo.chart" . }}
{{- end }}

{{- define "openchoreo-control-plane.labels" -}}
{{ include "openchoreo.labels" . }}
{{- end }}

{{- define "openchoreo-control-plane.selectorLabels" -}}
{{ include "openchoreo.selectorLabels" . }}
{{- end }}

{{- define "openchoreo-control-plane.serviceAccountName" -}}
{{ include "openchoreo.serviceAccountName" . }}
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
