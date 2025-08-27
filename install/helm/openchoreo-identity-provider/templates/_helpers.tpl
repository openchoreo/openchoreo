{{/*
Chart-specific aliases to openchoreo-common library functions
*/}}
{{- define "openchoreo-identity-provider.name" -}}
{{ include "openchoreo.name" . }}
{{- end }}

{{- define "openchoreo-identity-provider.fullname" -}}
{{ include "openchoreo.fullname" . }}
{{- end }}

{{- define "openchoreo-identity-provider.chart" -}}
{{ include "openchoreo.chart" . }}
{{- end }}

{{- define "openchoreo-identity-provider.labels" -}}
{{ include "openchoreo.labels" . }}
{{- end }}

{{- define "openchoreo-identity-provider.selectorLabels" -}}
{{ include "openchoreo.selectorLabels" . }}
{{- end }}

{{- define "openchoreo-identity-provider.serviceAccountName" -}}
{{ include "openchoreo.serviceAccountName" . }}
{{- end }}

{{/*
Component labels
Extends common labels with component-specific identification.
This should be used in the metadata.labels section of all component resources.

The component label (app.kubernetes.io/component) is used to identify different
components within the same application (e.g., keycloak, postgres, admin).

Usage:
  {{ include "openchoreo-identity-provider.componentLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-identity-provider.componentLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "keycloak", "postgres", "admin")
*/}}
{{- define "openchoreo-identity-provider.componentLabels" -}}
{{ include "openchoreo-identity-provider.labels" .context }}
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
  {{ include "openchoreo-identity-provider.componentSelectorLabels" (dict "context" . "component" "my-component") }}

Example with values:
  {{ include "openchoreo-identity-provider.componentSelectorLabels" (dict "context" . "component" .Values.myComponent.name) }}

Parameters:
  - context: The current Helm context (usually .)
  - component: The component name (e.g., "keycloak", "postgres", "admin")
*/}}
{{- define "openchoreo-identity-provider.componentSelectorLabels" -}}
{{ include "openchoreo-identity-provider.selectorLabels" .context }}
app.kubernetes.io/component: {{ .component }}
{{- end }}
