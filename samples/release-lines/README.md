# Release Lines

This sample builds two lines of the same component from two branches of a Git repository and runs them side by side in one environment, following [discussion #4702](https://github.com/openchoreo/openchoreo/discussions/4702).

Every component has an implicit `main` line, which is the component itself: its Workload, ComponentReleases and ReleaseBindings leave `spec.owner.releaseLine` empty, and nothing about them changes when other lines are added. A `ReleaseLine` adds another line next to it, with its own Workload, releases and binding per environment. Adding a line to an existing component needs no migration and no change to what already runs. `default` is reserved: no ReleaseLine can take that name.

The sample builds the [greeter service](https://github.com/openchoreo/sample-workloads/tree/main/service-go-greeter) of the OpenChoreo sample workloads, which reports its version at `/greeter/version`:

| Release line | Branch | Version |
|---|---|---|
| `main` (the component) | `main` | `v1.0` |
| `next` | `release-lines-next` | `v1.1` |

## Files

- `release-lines.yaml`: the `hello` component, whose own configuration is its `main` line, and the `next` release line. The `next` line overrides the branch to build and adds health probes.

The component uses the default `deployment/service` type and `dockerfile-builder` workflow, so the sample needs nothing else. The default component types name a line's Service, HTTPRoutes, backends and gateway paths from `metadata.componentLineName`, so the lines of a component never collide. The default build workflows publish each line's build to that line's own Workload, such as `hello-next-4833a175-workload`. A line's Workload, releases and bindings all start with `hello-next-4833a175`, so they never take the names of another component, such as one called `hello-next`.

## Deploy

```bash
kubectl apply -f samples/release-lines/release-lines.yaml
```

`occ apply -f samples/release-lines/release-lines.yaml` works too. It sends each ReleaseLine to the component named in its `spec.owner`.

## Build each line

```bash
occ component workflow run hello --namespace default
```

```bash
occ component workflow run hello --release-line next --namespace default
```

The first run builds the main line, the same as for any component, and `--release-line main` does the same. The second builds the `release-lines-next` branch. Each run creates or updates its line's Workload from the branch's `workload.yaml`, and the line deploys to the first environment of the pipeline.

```bash
kubectl get releaselines,workloads,releasebindings -n default
```

Each line keeps its own Workload. A line inherits the component's spec, meaning its parameters, traits and workflow parameters with the line's overrides applied, but not the main line's Workload. An environment variable added to `hello-workload` reaches the main line only, and one added to `hello-next-4833a175-workload` reaches `next` only. A build replaces its line's Workload from the branch's `workload.yaml`, so keep a line's environment variables in that file, or set them per environment under `workloadOverrides.container` in the line's ReleaseBinding.

## List the lines of a component

```bash
occ component releaseline list hello --namespace default
```

```bash
kubectl get releaselines -n default --field-selector spec.owner.componentName=hello
```

`occ` and the API server list `main` first, described from the bindings without a release line, then the ReleaseLines. `kubectl` shows only the ReleaseLines, since `main` has no object. `occ` lists one row per line and environment, with the bound release and whether the binding is ready. The API server serves the same list at `GET /api/v1/namespaces/default/components/hello/releaselines`.

## Manage a line

Show one line, including its status per environment:

```bash
occ component releaseline get hello next --namespace default
```

Create or change a line by editing it in a file and applying it with `occ apply -f`. An update replaces the line's whole spec.

Delete a line. This undeploys it from every environment and removes its releases and Workload. The component's other lines keep running.

```bash
occ component releaseline delete hello next --namespace default
```

The API server serves these under `/api/v1/namespaces/default/components/hello/releaselines/{line}` with `GET`, `PUT` and `DELETE`, and creates a line with `POST` on the list path. `main` can be read but not created, changed or deleted. Reading a line needs `releaseline:view`, and creating, changing or deleting one needs `releaseline:create`, `releaseline:update` or `releaseline:delete`. The default `developer` and `platform-engineer` roles have all four.

## Call each line

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-greeter-api/greeter/version
```

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-next-4833a175-greeter-api/greeter/version
```

The main line answers `v1.0` and `next` answers `v1.1`.

All lines render into the project's namespace. The main line's objects keep the names a component without lines gets. A release line's objects are named from `metadata.name` or `metadata.componentLineName`, so they never take the names of the main line or of another component. If two lines would render the same object, the binding reports `ReleaseLineNameCollision` and is not deployed.

A release line's pods carry the `openchoreo.dev/release-line` and `openchoreo.dev/release-line-uid` labels, and their component's UID as `openchoreo.dev/release-line-component-uid` instead of `openchoreo.dev/component-uid`, which the main line's selectors require. So the main line's Service and Deployment never select a release line's pods, and its selectors never change when lines are added. A component type must label pods with `metadata.podSelectors` for this to hold. A release line whose pods carry `openchoreo.dev/component-uid` reports `ReleaseLinePodLabelOverlap` and is not deployed.

## Consume a specific line

A dependency without `releaseLine` targets the component's main line, and keeps doing so when the component adds lines. To depend on another line, name it:

```yaml
dependencies:
  endpoints:
    - component: hello
      releaseLine: next
      name: greeter-api
      visibility: project
      envBindings:
        address: HELLO_URL
```

`occ remote` follows the same rule, so a workload running on your machine reaches the line its dependency names. When you also run a line of `hello` locally, `occ remote` wires the dependency to it, and `--local hello@next=127.0.0.1:9092` gives that line its own port.

## Promote a line

```bash
occ component deploy hello --release-line next --to staging --namespace default --project default
```

This binds the release that `next` runs in the previous environment to staging, as `hello-next-4833a175-staging`. The other lines are not touched.

## Roll a line back

```bash
occ component deploy hello --release-line next --to staging --release <earlier release of the line> --namespace default --project default
```

`--release` pins the binding to that release instead of promoting. To find an earlier release of the line, list the component's releases with their line, newest last:

```bash
kubectl get componentreleases -n default --sort-by=.metadata.creationTimestamp -o custom-columns=NAME:.metadata.name,LINE:.spec.owner.releaseLine,CREATED:.metadata.creationTimestamp
```

## Logs and exec

```bash
occ component logs hello --release-line next --namespace default --project default
```

```bash
occ component exec hello --release-line next --namespace default --project default
```

Without `--release-line`, `exec` runs in a pod of the main line and `logs` shows the main line's logs, as for a component without lines. The greeter logs each greeting and version request with its version, so each line's logs show which build served it. `logs --all-release-lines` shows the logs of every line.

## GitOps

In a GitOps repository, keep each ReleaseLine and its Workload next to the component, and generate releases and bindings in file-system mode. `--all` and `--project` cover every line of each component:

```bash
occ componentrelease generate --mode file-system --namespace default --all
```

```bash
occ releasebinding generate --mode file-system --all --use-pipeline default --target-env development
```

`--release-line` picks one line of `--component`, for example to promote `next` alone:

```bash
occ releasebinding generate --mode file-system --project default --component hello --release-line next --use-pipeline default --target-env staging
```

A line's releases are named `hello-next-<date>-<n>`. Its bindings are named `hello-next-<environment>`, as the control plane names them. A component whose main line has no Workload is released through its lines only.
