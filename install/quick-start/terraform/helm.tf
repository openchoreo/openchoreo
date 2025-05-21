resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "cilium"
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_choreo, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "choreo-dp" {
  name             = "choreo-dp"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "choreo-dp"
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

resource "helm_release" "choreo-cp" {
  name             = "choreo-cp"
  namespace        = var.namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "choreo-cp"
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}
