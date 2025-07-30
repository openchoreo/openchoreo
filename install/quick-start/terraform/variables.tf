variable "kubeconfig" {
  description = "Path to kubeconfig file"
  type        = string
  default     = "/state/kube/config-internal.yaml"
}

variable "cilium-namespace" {
  description = "Namespace to deploy Cilium Helm chart"
  type        = string
  default     = "cilium"
}

variable "control-plane-namespace" {
  description = "Namespace to deploy control plane Helm chart"
  type        = string
  default     = "openchoreo-control-plane"
}

variable "data-plane-namespace" {
  description = "Namespace to deploy data plane Helm chart"
  type        = string
  default     = "openchoreo-data-plane"
}

variable "observability-plane-namespace" {
  description = "Namespace to deploy observability plane Helm chart"
  type        = string
  default     = "openchoreo-observability-plane"
}

variable "build-plane-namespace" {
  description = "Namespace to deploy build plane Helm chart"
  type        = string
  default     = "openchoreo-build-plane"
}

variable "identity-provider-namespace" {
  description = "Namespace to deploy identity-provider Helm charts"
  type        = string
  default     = "openchoreo-identity-system"
}

variable "enable-observability-plane" {
  description = "Enable or disable the observability plane installation"
  type        = bool
  default     = false
}

variable "openchoreo_version" {
  description = "Version of OpenChoreo Helm charts to deploy (optional)"
  type        = string
  default     = null
}
