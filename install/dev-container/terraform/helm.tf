resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://choreov3testacr.azurecr.io/choreo-v3"
  chart           = "cilium"
  version         = var.cilium_version
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_choreo, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "choreo" {
  name             = "choreo"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://choreov3testacr.azurecr.io/choreo-v3"
  chart           = "choreo"
  version         = var.choreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}
