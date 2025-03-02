resource "kind_cluster" "kind_choreo" {
  name = "choreo"
  node_image = "kindest/node:v1.32.2"
  kind_config  {
    kind = "Cluster"
    api_version = "kind.x-k8s.io/v1alpha4"
    node {
      role = "control-plane"
    }
    node {
      role =  "worker"
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