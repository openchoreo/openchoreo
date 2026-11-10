# EndpointTypes for progressive delivery

These ClusterEndpointTypes render an Endpoint's weights, matches and mirrors from `${rules}`. Each one lists the routing features it renders in `spec.capabilities`, and an Endpoint that uses a feature its type doesn't list gets `Ready=False` with reason `UnsupportedByType`.

`${rules}` lists the match rules most specific first, as Gateway API ranks them: an exact path, then the longer path, then more headers and cookies, then the order of `backendRefs`. Rendered in that order, an `HTTPRoute` (which ranks by specificity) and a `VirtualService` or Envoy route (which take the first match) send a request to the same backend, so a type doesn't need to check the order itself, provided the gateway or mesh supports every feature the type renders.

The `HTTPRoute` samples use features Gateway API marks as extended, so the gateway must support them:

- regular expression header matches, for a header match of type `RegularExpression` and for every cookie match, which is rendered as a regular expression on the `Cookie` header;
- `URLRewrite` filters, which rewrite the path to each backend's base path;
- `RequestMirror` filters with `percent`, for mirrors.

A type declares in `capabilities` only what its implementation supports, so an Endpoint that needs more is refused instead of rendered with a feature dropped.

Every sample routes to each backend component's own Service, so it refuses a backend whose ComponentType renders none, and a type that renders an `HTTPRoute` refuses more than 16 rules, the most Gateway API allows in one route.

| Type | Renders | Callers | Capabilities |
|---|---|---|---|
| `http-progressive` | Gateway API `HTTPRoute` on the environment's gateway | external and internal | Match, RegularExpression, Mirror, MirrorPercent |
| `istio-virtualservice` | An Istio `VirtualService` for each backend Service | in-cluster, with Istio sidecars | Match, RegularExpression, Mirror, MirrorPercent |
| `gamma-httproute` | Gateway API `HTTPRoute` with the backends' Services as parents | in-cluster, with any GAMMA mesh | Match |
| `istio-http` | `HTTPRoute` on the external gateway, and an Istio `VirtualService` for each backend Service | external, and in-cluster (namespace, internal) with Istio sidecars | Match, RegularExpression, Mirror, MirrorPercent |
| `cilium-http` | `HTTPRoute` on the external gateway, and a `CiliumEnvoyConfig` | external, and in-cluster (namespace, internal) with Cilium's Envoy | Match, RegularExpression, Mirror, MirrorPercent |

`istio-http` and `cilium-http` serve one Endpoint to both directions with the same split: external callers through the gateway, and in-cluster callers through the mesh, at the addresses they already call. They expect the gateway outside the mesh, as the default data plane runs it, so each request is split once. `cilium-http` needs Cilium with `kubeProxyReplacement` and `envoyConfig.enabled`.

```sh
kubectl apply -f samples/progressive-delivery/endpoint-types/
```

A type for another gateway or mesh declares only what that implementation supports. For example, a mesh without request mirroring leaves out `Mirror`, so a mirror is refused instead of being silently dropped.
