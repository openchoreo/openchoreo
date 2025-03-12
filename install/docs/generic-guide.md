## Generic guide to Install Choreo using Helm Chart

### _Prerequisites_
- [Helm](https://helm.sh/docs/intro/install/) version v3.15+
  > Choreo use the Helm as the package manager to install the required artifacts into the kubernetes cluster.
- [Cilium](https://docs.cilium.io/en/stable/gettingstarted/k8s-install-default/#install-cilium) installed kubernetes cluster
    - Cilium version v1.15.10
    - kubernetes version v1.22.0+
  > Cilium is a dependency for choreo to operate. It uses the Cilium CNI plugin to manage the network policies and security for the pods in the cluster.


### Install

You can directly install Choreo using the Helm chart provided in our registry.

```shell
helm install choreo oci://ghcr.io/choreo-idp/helm-charts/choreo \
--version 0.1.0 --namespace "choreo-system" --create-namespace --timeout 30m
```