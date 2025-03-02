terraform {
  required_providers {
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.13"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }
    kind = {
      source = "tehcyx/kind"
      version = "0.8.0"
    }
  }

  required_version = ">= 1.3.0"
}

provider "helm" {
  kubernetes {
    config_path = var.kubeconfig
  }
}
