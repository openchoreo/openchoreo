variable "kubeconfig" {
  description = "Path to kubeconfig file"
  type        = string
  default     = "/state/kube/config-internal.yaml"
}

variable "namespace" {
  description = "Namespace to deploy Helm charts"
  type        = string
  default     = "choreo-system"
}

variable "cilium_version" {
  description = "Version of the Cilium Helm chart"
  type        = string
  default     = "0.1.0"
}

variable "choreo_version" {
  description = "Version of the Choreo Helm chart"
  type        = string
  default     = "0.1.0"
}
