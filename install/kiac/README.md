# kiac Setup for OpenChoreo

Local setup for OpenChoreo using [kiac](https://github.com/saiyam1814/kiac)
(Kubernetes in Apple Containers). Each node is its own lightweight VM on Apple's
`container` runtime, with a built-in LoadBalancer and no Docker Desktop.

## Prerequisites

- Apple silicon Mac, macOS 26+
- [apple/container](https://github.com/apple/container) 1.0.0+
- kiac 0.3.0+
- kubectl 1.32+
- Helm 3.12+ (the installer fetches a pinned Helm 3 if yours is newer)

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/openchoreo/openchoreo/main/install/kiac/kiac-install.sh \
  | bash -s -- --version <version>
```

This creates a kiac cluster and installs the OpenChoreo control plane and data
plane. When it finishes it prints the portal URL, an `/etc/hosts` line, and a
sample deploy command.

| Flag | Default | Description |
|------|---------|-------------|
| `--version` | `1.1.2` | OpenChoreo version to install |
| `--cluster-name` | `openchoreo` | kiac cluster name |
| `--workers` | `2` | worker node VMs |
| `--cni` | `kindnet` | `kindnet` or `cilium` (cilium enforces NetworkPolicies) |
| `--use-existing` | off | install into the current kiac cluster instead of creating one |

## Deploy a sample

```sh
kubectl --context kiac-openchoreo apply \
  -f https://raw.githubusercontent.com/openchoreo/openchoreo/main/samples/from-image/go-greeter-service/greeter-service.yaml
```

## Delete

```sh
kiac delete cluster --name openchoreo
```

## Notes

- kgateway watches the Gateway API `TLSRoute` type, so the installer applies the
  experimental Gateway API channel (a superset of the standard channel).
- Reachability uses kiac-lb's LoadBalancer IP; no host port mapping is required.
  Component endpoints are reachable from the host as-is; the portal additionally
  needs in-cluster DNS for `*.openchoreo.localhost` (see the summary the script prints).
- The workflow and observability planes are not yet supported by this installer.
