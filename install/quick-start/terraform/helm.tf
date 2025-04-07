resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "cilium"
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_choreo, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "choreo" {
  name             = "choreo"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "choreo"
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}
