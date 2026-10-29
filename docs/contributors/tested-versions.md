# Tested versions of third-party prerequisites

OpenChoreo relies on a few third-party components that are installed before
the OpenChoreo charts: the Gateway API CRDs, cert-manager, External Secrets
Operator (ESO), and kgateway. This page lists the version of each that a
release line is tested against.

The **tested minor version** is what OpenChoreo validates. The installers,
quick-start, and e2e suite pin one patch release of that minor, shown in the
"Pinned" column. You can apply newer patch releases of a tested minor version
(for example, a security fix) without waiting for an OpenChoreo release. A
different minor version has not been tested with that release line.

## v1.3

Release branch: `release-v1.3`

| Prerequisite              | Tested minor | Pinned  | Notes |
|---------------------------|--------------|---------|---|
| Gateway API CRDs          | v1.5         | v1.5.1  | The k3d installers, quick-start, and e2e suite apply the Standard channel (`standard-install.yaml`). The kiac installer applies the Experimental channel (`experimental-install.yaml`) instead. |
| cert-manager              | v1.19        | v1.19.4 | Installed from the `oci://quay.io/jetstack/charts/cert-manager` Helm chart. |
| External Secrets Operator | 2.0          | 2.0.1   | This is the Helm chart version (`oci://ghcr.io/external-secrets/charts/external-secrets`), passed to Helm without a `v` prefix. Quick-start stores it as `v2.0.1` and strips the prefix before calling Helm. |
| kgateway                  | v2.3         | v2.3.1  | The `kgateway-crds` and `kgateway` Helm charts are installed at the same version. |

## How this is maintained

Each `release-vX.Y` branch carries its own copy of this page, and the table on
that branch describes that release line. When a PR bumps a prerequisite on a
release branch, update the table on that branch in the same PR. When a new
release line is cut, add a `## vX.Y` section for it above the previous one.

The pinned versions are set by hand in these files, which must agree with the
table:

- [`install/k3d/k3d-install.sh`](../../install/k3d/k3d-install.sh)
- [`install/k3d/single-cluster/README.md`](../../install/k3d/single-cluster/README.md)
- [`install/k3d/multi-cluster/README.md`](../../install/k3d/multi-cluster/README.md)
- [`install/quick-start/.config.sh`](../../install/quick-start/.config.sh) and
  the Gateway API CRDs URL in
  [`install/quick-start/.helpers.sh`](../../install/quick-start/.helpers.sh)
- [`install/kiac/kiac-install.sh`](../../install/kiac/kiac-install.sh)
- [`make/e2e.mk`](../../make/e2e.mk)
