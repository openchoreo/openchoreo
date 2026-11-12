# Progressive delivery with Endpoints

An Endpoint routes to one or more backends. A rollout moves its traffic from the backend serving now (`stable`) to a new one (the `candidate`), one environment at a time:

- **The plan** is `spec.rollout` on the Endpoint: the strategy, the two backends, and the steps. It is not part of a release, so editing it cuts none.
- **Each step** writes the weights on one environment's EndpointReleaseBinding (`spec.backendOverrides`). It cuts no release either, and the other environments keep their own split.
- **Any client** carries out the plan the same way: `occ endpoint rollout`, the MCP tools, openchoreo-api directly, or the portal once its Endpoint rollout controls ship.

| Sample | Strategy | Question it answers | Story |
| --- | --- | --- | --- |
| [mirror](mirror/) | Copy a share of requests to the candidate, whose answers are dropped | Does the new code survive real traffic before anyone depends on it? | Dark launching the trips rewrite |
| [canary](canary/) | A growing share: 5, 25, 50, 100% | Is the new code safe? | Uber rewriting a Python service in Go |
| [blue-green](blue-green/) | A header preview, then everyone at once | Can every request change at the same moment? | The EU VAT rules of 1 July 2021 |
| [ab-test](ab-test/) | The requests carrying a cookie group | Is the new feature better? | Amazon's recommendations in the cart |

The mirror and canary samples share the trips Endpoint: shadow the rewrite first, then roll it out.

## Before you start

You need the local k3d setup with Istio on the data plane, and `occ` logged in (`occ login`).

```sh
kubectl apply -f samples/progressive-delivery/endpoint-types/
make -C samples/progressive-delivery/app images
```

The samples use the [`istio-http`](endpoint-types/istio-http.yaml) type: external callers come through the gateway and in-cluster callers through the mesh, with the same split. [`endpoint-types/`](endpoint-types/) also has types for Gateway API alone, Cilium and GAMMA.

The [app](app/) is one small Go server that plays both versions in every sample. Every answer names its backend in the `X-Served-By` header.

## Measuring a split

[`tally.sh`](tally.sh) sends requests both ways and counts who answered:

```sh
./samples/progressive-delivery/tally.sh trips              # 100 each way
N=200 ./samples/progressive-delivery/tally.sh trips -H 'x-canary: true'
```

- Through the gateway, it calls the URL in the binding's `status.urls.external`.
- In-cluster, it calls the stable backend's own address from a pod with a sidecar. The mesh splits that address too, so callers never change the address they use.

Expect the counts to wobble by a few percent around the step's share at 100 requests.

## Cleaning up

```sh
kubectl delete -f samples/progressive-delivery/canary/endpoint.yaml -f samples/progressive-delivery/blue-green/endpoint.yaml \
  -f samples/progressive-delivery/ab-test/endpoint.yaml --ignore-not-found
kubectl delete -f samples/progressive-delivery/canary/components.yaml -f samples/progressive-delivery/blue-green/components.yaml \
  -f samples/progressive-delivery/ab-test/components.yaml --ignore-not-found
```

Delete the Endpoints first, so no component is deleted while it still gets traffic.
