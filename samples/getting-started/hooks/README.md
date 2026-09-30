# Deployment hooks sample (alpha)

Not part of `all.yaml`. Requires a workflow plane; the control plane always
evaluates hooks bound on an Environment.

```bash
kubectl apply -f samples/getting-started/hooks/trivy-image-scan-workflow.yaml
kubectl apply -f samples/getting-started/hooks/trivy-image-scan-hook.yaml
kubectl apply -f samples/getting-started/hooks/production-environment-with-hooks.yaml   # replaces the production environment
```

What it does: every deployment of a `service` component into `production` first runs
Trivy against the release's `main` container image. CRITICAL and HIGH findings fail the scan and the
release is **not** rendered; the currently running release stays as it is.

Reading the result:

```bash
kubectl get releasebinding -n default <component>-production -o jsonpath='{.status.gate}' | jq
kubectl get workflowrun -n default -l openchoreo.dev/workflow-purpose=deployment-hook
```

Note that a ReleaseBinding whose `ReleaseSynced` condition is true is not necessarily
deployed: while `PreDeployHooksPassed` is `False`, `Ready` is `False` and no
`RenderedRelease` exists for the new release. Retry a failed scan with:

```bash
kubectl annotate releasebinding -n default <component>-production openchoreo.dev/hook-retry=preDeploy/image-scan
```

See [docs/deployment-hooks.md](../../../docs/deployment-hooks.md).
