# EndpointTypes for progressive delivery

These ClusterEndpointTypes render an Endpoint's weights, matches and mirrors from `${rules}`. Each one lists the routing features it renders in `spec.capabilities`, and an Endpoint that uses a feature its type doesn't list gets `Ready=False` with reason `UnsupportedByType`.

| Type | Renders | Callers | Capabilities |
|---|---|---|---|
| `http-progressive` | Gateway API `HTTPRoute` on the environment's gateway | external and internal | Match, RegularExpression, Mirror, MirrorPercent |
| `istio-virtualservice` | Istio `VirtualService` on the backends' Services | in-cluster, with Istio sidecars | Match, RegularExpression, Mirror, MirrorPercent |
| `gamma-httproute` | Gateway API `HTTPRoute` with the backends' Services as parents | in-cluster, with any GAMMA mesh | Match |
| `istio-http` | `HTTPRoute` on the external gateway, and an Istio `VirtualService` | external, and in-cluster (namespace, internal) with Istio sidecars | Match, RegularExpression, Mirror, MirrorPercent |
| `cilium-http` | `HTTPRoute` on the external gateway, and a `CiliumEnvoyConfig` | external, and in-cluster (namespace, internal) with Cilium's Envoy | Match, RegularExpression, Mirror, MirrorPercent |

`istio-http` and `cilium-http` serve one Endpoint to both directions with the same split: external callers through the gateway, and in-cluster callers through the mesh, at the addresses they already call. They expect the gateway outside the mesh, as the default data plane runs it, so each request is split once. `cilium-http` needs Cilium with `kubeProxyReplacement` and `envoyConfig.enabled`.

```sh
kubectl apply -f samples/progressive-delivery/endpoint-types/
```

A type for another gateway or mesh declares only what that implementation supports. For example, a mesh without request mirroring leaves out `Mirror`, so a mirror is refused instead of being silently dropped.
