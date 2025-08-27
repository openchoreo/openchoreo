{{/*
Chart-specific aliases to openchoreo-common library functions
*/}}
{{- define "openchoreo-backstage.name" -}}
{{ include "openchoreo.name" . }}
{{- end }}

{{- define "openchoreo-backstage.fullname" -}}
{{ include "openchoreo.fullname" . }}
{{- end }}

{{- define "openchoreo-backstage.chart" -}}
{{ include "openchoreo.chart" . }}
{{- end }}

{{- define "openchoreo-backstage.labels" -}}
{{ include "openchoreo.labels" . }}
{{- end }}

{{- define "openchoreo-backstage.selectorLabels" -}}
{{ include "openchoreo.selectorLabels" . }}
{{- end }}

{{- define "openchoreo-backstage.serviceAccountName" -}}
{{ include "openchoreo.serviceAccountName" . }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., frontend, api, worker).

Usage:
  {{ include "openchoreo-backstage.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-backstage.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "frontend", "api", "worker")
*/}}
{{- define "openchoreo-backstage.componentLabels" -}}
{{ include "openchoreo-backstage.labels" .context }}
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
  {{ include "openchoreo-backstage.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-backstage.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "frontend", "api", "worker")
*/}}
{{- define "openchoreo-backstage.componentSelectorLabels" -}}
{{ include "openchoreo-backstage.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}
