resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/choreo-idp/helm-charts"
  chart           = "cilium"
  version         = var.cilium_version
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_choreo, null_resource.connect_container_to_kind_network]

  set {
    name  = "waitJob.enabled"
    value = "false"
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "helm_release" "choreo" {
  name             = "choreo"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/choreo-idp/helm-charts"
  chart           = "choreo"
  version         = var.choreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]

  lifecycle {
    prevent_destroy = true
  }
}
