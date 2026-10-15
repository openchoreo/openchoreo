# Release Lines

This sample builds two lines of the same component from two branches of a Git repository and runs them side by side in one environment, following [discussion #4702](https://github.com/openchoreo/openchoreo/discussions/4702).

A component either does not use release lines, which is how every component works today, or it opts in with `spec.releaseLinesEnabled: true` at creation. An opted-in component has no implicit line: every Workload, ComponentRelease, and ReleaseBinding names a `ReleaseLine`. The flag cannot be changed later, so an existing component adopts release lines by migrating to a new component.

The sample uses [go-greeter](https://github.com/chathuranga95/go-greeter):

| Release line | Branch | Response version |
|---|---|---|
| `main` | `main` | `v1.0` |
| `next` | `next` | `v1.1` |

## Files

- `component-type.yaml`: the `release-line-service` ClusterComponentType. It is the default `service` type with component-scoped objects (Service, HTTPRoutes, backends, gateway paths) named from `metadata.releaseLineName`, so the lines of a component never collide.
- `build-workflow.yaml`: the `release-line-dockerfile-builder` ClusterWorkflow. It is the default dockerfile builder, except that it passes the run's release line to the workload generation step.
- `generate-workload-template.yaml`: the `release-line-generate-workload` ClusterWorkflowTemplate. It is the default workload generation step, except that it publishes the built image to the run's release line, as the Workload `<component>-<release-line>-workload`.
- `release-lines.yaml`: the component and its two release lines. The `next` line overrides only the branch to build.

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
occ component workflow run hello --release-line main --namespace default
```

```bash
occ component workflow run hello --release-line next --namespace default
```

Each run builds the line's branch and creates or updates that line's Workload from the branch's `workload.yaml`. The component then deploys each line to the first environment of the pipeline.

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

Both show each line's environments. `occ` lists one row per line and environment, with the bound release and whether the binding is ready. The API server serves the same list at `GET /api/v1/namespaces/default/components/hello/releaselines`.

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

The API server serves these under `/api/v1/namespaces/default/components/hello/releaselines/{line}` with `GET`, `PUT` and `DELETE`, and creates a line with `POST` on the list path. Reading a line needs `component:view`, and creating, changing or deleting one needs `component:update`.

## Call each line

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-main-http/greet
```

```bash
curl http://development-default.openchoreoapis.localhost:19080/hello-next-http/greet
```

All lines render into the project's namespace. Each line's objects are named from `metadata.name` or `metadata.releaseLineName`, and its pods carry an `openchoreo.dev/release-line` label. If two lines would render the same object, the binding reports `ReleaseLineNameCollision` and is not deployed.

## Consume a specific line

A dependency on a component with release lines must name the line:

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

`exec` requires `--release-line` for a component with release lines. `logs` without it shows the logs of every line.
