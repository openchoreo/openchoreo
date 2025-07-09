resource "kind_cluster" "kind_choreo" {
  name = "choreo-quick-start"
  node_image = "kindest/node:v1.32.0@sha256:c48c62eac5da28cdadcf560d1d8616cfa6783b58f0d94cf63ad1bf49600cb027"
  kind_config  {
    kind = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"
    node {
      role = "control-plane"
    }
    node {
      role =  "worker"
      labels = {
          "openchoreo.dev/noderole" = "workflow-runner"
      }
      extra_mounts {
        host_path = "/tmp/kind-shared"
        container_path = "/mnt/shared"
      }
    }
    networking {
      disable_default_cni = "true"
    }
  }
  kubeconfig_path = "/state/kube/config.yaml"

  depends_on = [terraform_data.create_kube_dir]
}
