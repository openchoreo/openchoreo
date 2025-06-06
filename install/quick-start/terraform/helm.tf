resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "cilium"
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_choreo, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "choreo-dataplane" {
  name             = "choreo-dataplane"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "choreo-dataplane"
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
  set {
        name  = "certmanager.enabled"
        value = "false"
  }
  set {
        name  = "certmanager.crds.enabled"
        value = "false"
  }
}

resource "helm_release" "choreo-control-plane" {
  name             = "choreo-control-plane"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "choreo-control-plane"
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}
