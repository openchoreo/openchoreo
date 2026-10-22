# Release Lines

This sample builds two lines of the same component from two branches of a Git repository and runs them side by side in one environment, following [discussion #4702](https://github.com/openchoreo/openchoreo/discussions/4702).

Every component has an implicit `default` line, which is the component itself: its Workload, ComponentReleases and ReleaseBindings leave `spec.owner.releaseLine` empty, and nothing about them changes when other lines are added. A `ReleaseLine` adds another line next to it, with its own Workload, releases and binding per environment. Adding a line to an existing component needs no migration and no change to what already runs. `default` is reserved: no ReleaseLine can take that name.

The component's type must allow release lines with `spec.releaseLines: Allowed`. Otherwise its ReleaseLines are not released and report `ReleaseLinesNotAllowed`. The default component types allow them.

The sample uses [go-greeter](https://github.com/chathuranga95/go-greeter):

| Release line | Branch | Response version |
|---|---|---|
| `default` (the component) | `main` | `v1.0` |
| `next` | `next` | `v1.1` |

## Files

- `component-type.yaml`: the `release-line-service` ClusterComponentType. It is the default `service` type, which already names component-scoped objects (Service, HTTPRoutes, backends, gateway paths) from `metadata.componentName` and `metadata.releaseLineName` so the lines of a component never collide, plus a `logLevel` parameter for the `next` line to override.
- `build-workflow.yaml`: the `release-line-dockerfile-builder` ClusterWorkflow. It is the default dockerfile builder, except that it passes the run's release line to the workload generation step.
- `generate-workload-template.yaml`: the `release-line-generate-workload` ClusterWorkflowTemplate. It is the default workload generation step, except that it publishes the built image to the run's release line, as the Workload `<component>-<release-line>-workload`.
- `release-lines.yaml`: the component, whose own configuration is its `default` line, and the `next` release line. The `next` line overrides the branch to build, the log level and the health probes.

## Deploy

```bash
kubectl apply -f samples/release-lines/component-type.yaml -f samples/release-lines/build-workflow.yaml -f samples/release-lines/generate-workload-template.yaml
```

On a multi-cluster install, the ClusterWorkflowTemplate belongs on the workflow plane cluster and the rest on the control plane cluster. With the k3d multi-cluster setup:

```bash
kubectl --context k3d-openchoreo-cp apply -f samples/release-lines/component-type.yaml -f samples/release-lines/build-workflow.yaml
```

```bash
kubectl --context k3d-openchoreo-wp apply -f samples/release-lines/generate-workload-template.yaml
```

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

The first run builds the default line, the same as for any component, and `--release-line default` does the same. The second builds the `next` branch. Each run creates or updates its line's Workload from the branch's `workload.yaml`, and the line deploys to the first environment of the pipeline.

```bash
kubectl get releaselines,workloads,releasebindings -n default
```

## List the lines of a component

```bash
occ component releaseline list hello --namespace default
```

```bash
kubectl get releaselines -n default --field-selector spec.owner.componentName=hello
```

`occ` and the API server list `default` first, described from the bindings without a release line, then the ReleaseLines. `kubectl` shows only the ReleaseLines, since `default` has no object. `occ` lists one row per line and environment, with the bound release and whether the binding is ready. The API server serves the same list at `GET /api/v1/namespaces/default/components/hello/releaselines`.

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

The API server serves these under `/api/v1/namespaces/default/components/hello/releaselines/{line}` with `GET`, `PUT` and `DELETE`, and creates a line with `POST` on the list path. `default` can be read but not created, changed or deleted. Reading a line needs `releaseline:view`, and creating, changing or deleting one needs `releaseline:create`, `releaseline:update` or `releaseline:delete`. The default `developer` and `platform-engineer` roles have all four.

## Call each line

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-http/greet
```

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-next-http/greet
```

All lines render into the project's namespace. The default line's objects keep the names a component without lines gets. A release line's objects are named from `metadata.name`, or from `metadata.componentName` and `metadata.releaseLineName`, so they never take the default line's names. If two lines would render the same object, the binding reports `ReleaseLineNameCollision` and is not deployed.

A release line's pods carry the `openchoreo.dev/release-line` and `openchoreo.dev/release-line-uid` labels, and their component's UID as `openchoreo.dev/release-line-component-uid` instead of `openchoreo.dev/component-uid`, which the default line's selectors require. So the default line's Service and Deployment never select a release line's pods, and its selectors never change when lines are added. A component type must label pods with `metadata.podSelectors` for this to hold. A release line whose pods carry `openchoreo.dev/component-uid` reports `ReleaseLinePodLabelOverlap` and is not deployed.

## Consume a specific line

A dependency without `releaseLine` targets the component's default line, and keeps doing so when the component adds lines. To depend on another line, name it:

```yaml
dependencies:
  endpoints:
    - component: hello
      releaseLine: next
      name: http
      visibility: project
      envBindings:
        address: HELLO_URL
```

`occ remote` follows the same rule, so a workload running on your machine reaches the line its dependency names. When you also run a line of `hello` locally, `occ remote` wires the dependency to it, and `--local hello@next=127.0.0.1:9092` gives that line its own port.

## Promote a line

```bash
occ component deploy hello --release-line next --to staging --namespace default --project default
```

This binds the release that `next` runs in the previous environment to staging, as `hello-next-staging`. The other lines are not touched.

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

`exec` without `--release-line` runs in a pod of the default line. `logs` without it shows the logs of every line, and `--release-line default` shows the default line's only.

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

A line's releases are named `hello-next-<date>-<n>`. Its bindings are named `hello-next-<environment>`, as the control plane names them. A component whose default line has no Workload is released through its lines only.
