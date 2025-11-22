{{/*
Validation for LLM configuration
This template validates that required LLM configuration is provided based on the selected provider
*/}}

{{- define "openchoreo-observer-rca.validateLLMConfig" -}}
{{- $provider := .Values.rcaService.llm.provider -}}

{{/* Validate provider value */}}
{{- $validProviders := list "openai" "anthropic" "azureopenai" "googlegenai" -}}
{{- if not (has $provider $validProviders) -}}
  {{- fail (printf "\n\nERROR: Unsupported provider '%s'.\n\nValid providers are: %s\n\nSet the provider using:\n  --set rcaService.llm.provider=\"<provider>\"\n" $provider (join ", " $validProviders)) -}}
{{- end -}}

{{/* Validate model name is provided */}}
{{- if not .Values.rcaService.llm.modelName -}}
  {{- fail "\n\nERROR: rcaService.llm.modelName is required.\n\nProvide the model name using:\n  --set rcaService.llm.modelName=\"your-model-name\"\n\nExamples:\n  Anthropic: claude-sonnet-4-5, claude-3-5-sonnet-20241022\n  OpenAI: gpt-5, gpt-4-turbo\n  Azure OpenAI: (same as azure.deployment)\n  Google: gemini-2.0-flash-exp, gemini-1.5-pro\n" -}}
{{- end -}}

{{/* Validate API key is provided when not using external secrets */}}
{{- if not .Values.rcaService.llm.externalSecret.enabled -}}
  {{- if not .Values.rcaService.llm.apiKey -}}
    {{- fail "\n\nERROR: rcaService.llm.apiKey is required when external secrets are not enabled.\n\nProvide the API key using:\n  --set rcaService.llm.apiKey=\"your-api-key\"\n\nOr enable external secrets:\n  --set rcaService.llm.externalSecret.enabled=true\n" -}}
  {{- end -}}
{{- end -}}

{{/* Validate external secret configuration */}}
{{- if .Values.rcaService.llm.externalSecret.enabled -}}
  {{- if not .Values.rcaService.llm.externalSecret.secretStoreRef -}}
    {{- fail "\n\nERROR: rcaService.llm.externalSecret.secretStoreRef is required when external secrets are enabled.\n\nProvide the secret store reference using:\n  --set rcaService.llm.externalSecret.secretStoreRef=\"your-secret-store\"\n" -}}
  {{- end -}}
  {{- if not .Values.rcaService.llm.externalSecret.apiKeyRef.key -}}
    {{- fail "\n\nERROR: rcaService.llm.externalSecret.apiKeyRef.key is required when external secrets are enabled.\n\nProvide the secret key reference using:\n  --set rcaService.llm.externalSecret.apiKeyRef.key=\"path/to/secret\"\n" -}}
  {{- end -}}
{{- end -}}

{{/* Validate provider-specific configuration for Azure OpenAI */}}
{{- if eq $provider "azureopenai" -}}
  {{- if not .Values.rcaService.llm.azure.endpoint -}}
    {{- fail (printf "\n\nERROR: rcaService.llm.azure.endpoint is required when provider is 'azureopenai'.\n\nProvide the Azure OpenAI endpoint using:\n  --set rcaService.llm.azure.endpoint=\"https://your-resource.openai.azure.com\"\n") -}}
  {{- end -}}
  {{- if not .Values.rcaService.llm.azure.apiVersion -}}
    {{- fail (printf "\n\nERROR: rcaService.llm.azure.apiVersion is required when provider is 'azureopenai'.\n\nProvide the API version using:\n  --set rcaService.llm.azure.apiVersion=\"2024-02-15-preview\"\n") -}}
  {{- end -}}
  {{- if not .Values.rcaService.llm.azure.deployment -}}
    {{- fail (printf "\n\nERROR: rcaService.llm.azure.deployment is required when provider is 'azureopenai'.\n\nProvide the deployment name using:\n  --set rcaService.llm.azure.deployment=\"your-deployment-name\"\n") -}}
  {{- end -}}
{{- end -}}

{{- end -}}
