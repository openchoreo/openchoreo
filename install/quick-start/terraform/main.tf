resource "terraform_data" "create_kube_dir" {
  provisioner "local-exec" {
    command = "mkdir -p /state/kube"
  }
}

resource "null_resource" "connect_container_to_kind_network" {
  provisioner "local-exec" {
    command = "./scripts/docker_connect.sh"
  }
  depends_on = [kind_cluster.kind_choreo]
}

resource "terraform_data" "export_kubeconfig" {
  provisioner "local-exec" {
    command = "kind export kubeconfig --internal -n choreo-quick-start --kubeconfig ${var.kubeconfig}"
  }
  depends_on = [kind_cluster.kind_choreo]
}
