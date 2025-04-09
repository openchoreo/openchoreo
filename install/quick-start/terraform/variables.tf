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
