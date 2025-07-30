resource "helm_release" "cilium" {
  name             = "cilium"
  namespace        = var.cilium-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "cilium"
  version         = var.openchoreo_version
  timeout         = 1800 # 30 minutes
  depends_on = [kind_cluster.kind_openchoreo, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "openchoreo-data-plane" {
  name             = "openchoreo-data-plane"
  namespace        = var.data-plane-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "openchoreo-data-plane"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
  set {
        name  = "cert-manager.enabled"
        value = "false"
  }
  set {
        name  = "cert-manager.crds.enabled"
        value = "false"
  }
}

resource "helm_release" "openchoreo-control-plane" {
  name             = "openchoreo-control-plane"
  namespace        = var.control-plane-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "openchoreo-control-plane"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "openchoreo-build-plane" {
  name             = "openchoreo-build-plane"
  namespace        = var.build-plane-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "openchoreo-build-plane"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "openchoreo-observability-plane" {
  count            = var.enable-observability-plane ? 1 : 0
  name             = "openchoreo-observability-plane"
  namespace        = var.observability-plane-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "openchoreo-observability-plane"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "openchoreo-identity-provider" {
  name             = "openchoreo-identity-provider"
  namespace        = var.identity-provider-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "openchoreo-identity-provider"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
}

resource "helm_release" "openchoreo-backstage-demo" {
  name             = "openchoreo-backstage-demo"
  namespace        = var.control-plane-namespace
  create_namespace = true
  repository       = "oci://ghcr.io/openchoreo/helm-charts"
  chart           = "backstage-demo"
  version         = var.openchoreo_version
  wait            = false
  timeout         = 1800 # 30 minutes
  depends_on = [helm_release.cilium, null_resource.connect_container_to_kind_network]
  set {
    name  = "backstage.service.type"
    value = "NodePort"
  }
}
