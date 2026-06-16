# GitHub Actions Integration

OpenChoreo integrates with GitHub Actions as an external CI platform. Once enabled, the
Backstage developer portal can read `workflow_run` data for any Component that carries
the `github.com/project-slug` annotation, and (in a follow-up release) workflows running
on GitHub Actions can register a new Workload back into OpenChoreo at the end of a build.

This document covers the **portal-side wiring** that ships in the `openchoreo-control-plane`
Helm chart. The **deployment bridge** (reusable workflow + OIDC validation) is tracked in
the linked follow-up and will be documented in this file when it lands.

> **Two GitHub credentials are involved.** The optional backend token in sections 4–5
> (`github-actions-token`) is consumed only by Backstage **backend** plugins (catalog
> ingestion, scaffolder, TechDocs). The **GitHub Actions card itself authenticates each
> user via per-user GitHub OAuth**, which requires a separate GitHub **OAuth App** and an
> `auth.providers.github` block (section 1). Without it the card cannot fetch runs, even
> with a valid PAT.
> Related: [#3551](https://github.com/openchoreo/openchoreo/issues/3551) — follow-up to
> [#1788](https://github.com/openchoreo/openchoreo/issues/1788).

## Architecture

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                  PLATFORM ENGINEER (one-time setup)                          │
├─────────────────────────────────────────────────────────────────────────────┤
│  1. Create a GitHub OAuth App and register the `github` auth provider —     │
│     this is what powers the per-user sign-in the Actions card uses.         │
│  2. Enable the integration via Helm values.                                 │
│  3. Provision a GitHub PAT (or App installation token) with `repo`          │
│     and `actions:read` scopes (optional, backend only).                     │
│  4. Add it to the Backstage credentials Secret under `github-actions-token`.│
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DEVELOPER WORKFLOW                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  Annotate the Component:                                                    │
│    metadata:                                                                │
│      annotations:                                                           │
│        github.com/project-slug: <org>/<repo>                                │
│                                                                              │
│  The Component page in Backstage now surfaces a "GitHub Actions" tab        │
│  showing recent workflow_runs for that repository.                          │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 1. Enable per-user GitHub sign-in (required for the Actions card)

The GitHub Actions card does **not** use the optional backend PAT (sections 4–5). It
authenticates each portal user individually through GitHub OAuth (`ScmAuth`) and then calls
the GitHub API with that per-user token, requesting the `repo` scope — so a user who can see
a private repository on GitHub can see its runs in the portal. Until a GitHub OAuth provider
is registered, opening the card fails and `GET /api/auth/github/start` returns:

```text
NotFoundError: No auth provider registered for 'github'   (HTTP 404)
```

### 1.1 Create a GitHub OAuth App

GitHub → **Settings → Developer settings → OAuth Apps → New OAuth App** (use an
organization-owned App for shared portals):

| Field                      | Value                                                                                              |
| -------------------------- | -------------------------------------------------------------------------------------------------- |
| Homepage URL               | your portal base URL, e.g. `http://openchoreo.localhost:8080`                                      |
| Authorization callback URL | `<portal-base-url>/api/auth/github/handler/frame`, e.g. `http://openchoreo.localhost:8080/api/auth/github/handler/frame` |

Record the **Client ID** and generate a **Client secret**. For GitHub Enterprise Server,
create the OAuth App on your GHES instance instead of github.com.

> The callback URL must match the portal base URL exactly (scheme, host, port) or GitHub
> rejects the login with a `redirect_uri` mismatch.

### 1.2 Register the `github` auth provider

Add a `github` provider under `auth.providers`, keyed by the active `auth.environment`
(the chart default is `development`). This mirrors the existing `openchoreo-auth`
provider — the values come from environment variables injected into the Backstage
Deployment, so the secret never lands in a ConfigMap:

```yaml
auth:
  environment: development      # already set by the chart
  providers:
    github:
      development:
        clientId: ${AUTH_GITHUB_CLIENT_ID}
        clientSecret: ${AUTH_GITHUB_CLIENT_SECRET}
```

The chart renders this block automatically when you set
`backstage.externalCI.githubActions.oauth.clientId` (see section 3). Store the credentials
so the Deployment can source them.

**If `backstage-secrets` is managed by an ExternalSecret** (the default OpenBao-backed
install), **do not patch the Kubernetes Secret directly** — External Secrets Operator owns
it (`creationPolicy: Owner`) and overwrites it on the next sync. Seed the values into the
secret backend and map them through the ExternalSecret instead.

1. Seed the OpenBao KV. Paths follow the `secret/backstage-<secretKey>` convention used by
   the other Backstage secrets, with the value under the `value` property:

   ```bash
   kubectl exec -n openbao openbao-0 -- sh -c '
     export BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root
     bao kv put secret/backstage-github-oauth-client-id     value="<your-client-id>"
     bao kv put secret/backstage-github-oauth-client-secret value="<your-client-secret>"
   '
   ```

   > The client ID is not sensitive, but the client secret is — never paste it inline on
   > the command line (it lands in shell history and in the `kubectl exec` process args,
   > visible via `ps`/audit logs). Read it into a variable first, as above, and rotate any
   > secret exposed that way.

2. Add the two keys to the `backstage-secrets` ExternalSecret `spec.data` (the
   `secretKey` becomes the Kubernetes Secret key the Deployment reads). Append them with
   a JSON patch so the existing entries are left untouched:

   ```bash
   kubectl patch externalsecret backstage-secrets -n openchoreo-control-plane --type=json -p='[
     {"op":"add","path":"/spec/data/-","value":{"secretKey":"github-oauth-client-id","remoteRef":{"key":"backstage-github-oauth-client-id","property":"value"}}},
     {"op":"add","path":"/spec/data/-","value":{"secretKey":"github-oauth-client-secret","remoteRef":{"key":"backstage-github-oauth-client-secret","property":"value"}}}
   ]'
   ```

   `/spec/data/-` appends, so run this **once** — re-running duplicates the entries. This
   adds the equivalent of:

   ```yaml
   - secretKey: github-oauth-client-id
     remoteRef:
       key: backstage-github-oauth-client-id
       property: value
   - secretKey: github-oauth-client-secret
     remoteRef:
       key: backstage-github-oauth-client-secret
       property: value
   ```

3. Force a re-sync rather than waiting for `refreshInterval: 1h`, then confirm it synced:

   ```bash
   kubectl annotate externalsecret backstage-secrets -n openchoreo-control-plane \
     force-sync=$(date +%s) --overwrite
   ```

**For installs that create `backstage-secrets` directly** (no ExternalSecret / non-OpenBao
backend), add the keys to that Secret instead. Export the values first so the commands are
copy-paste-safe:

```bash
export GH_OAUTH_CLIENT_ID="<your-client-id>"
export GH_OAUTH_CLIENT_SECRET="<your-client-secret>"

kubectl -n openchoreo-control-plane patch secret backstage-secrets --type=json -p='[
  {"op":"add","path":"/data/github-oauth-client-id","value":"'"$(printf %s "$GH_OAUTH_CLIENT_ID" | base64 | tr -d '\n')"'"},
  {"op":"add","path":"/data/github-oauth-client-secret","value":"'"$(printf %s "$GH_OAUTH_CLIENT_SECRET" | base64 | tr -d '\n')"'"}
]'
```

After applying, restart Backstage and confirm the provider is registered (the image has
no `curl`, so use the bundled `node`):

```bash
kubectl rollout restart deployment -n openchoreo-control-plane \
  -l app.kubernetes.io/component=backstage

POD=$(kubectl get pod -n openchoreo-control-plane \
  -l app.kubernetes.io/component=backstage \
  --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[-1:].metadata.name}')

# Expect HTTP 302 to github.com/login/oauth/authorize — NOT 404 "No auth provider registered"
kubectl exec -n openchoreo-control-plane "$POD" -- node -e '
  require("http").get("http://127.0.0.1:7007/api/auth/github/start?env=development",
    r => console.log(r.statusCode, r.headers.location || ""));'
```

Then open the portal, go to a Component's **CI/CD** tab, and complete the one-time GitHub
login when the popup appears. The card then lists recent workflow runs.

## 2. Annotate Components

For any Component whose workflows you want to see in Backstage, set the
`github.com/project-slug` annotation:

```yaml
apiVersion: openchoreo.dev/v1alpha1
kind: Component
metadata:
  name: my-service
  annotations:
    github.com/project-slug: my-org/my-service
spec:
  # ...
```

This is the same annotation that the upstream [`@backstage/plugin-github-actions`](https://www.npmjs.com/package/@backstage/plugin-github-actions)
plugin reads, so any existing Backstage docs that reference it apply unchanged.

> The card reads this annotation from the **Backstage catalog entity**, not from the
> OpenChoreo Component CR directly. If your catalog sync does not copy the CR annotation
> onto the entity, set `github.com/project-slug` on the catalog entity itself. Confirm
> it is present with:
>
> ```bash
> kubectl exec -n openchoreo-control-plane "$POD" -- node -e '
>   require("http").get("http://127.0.0.1:7007/api/catalog/entities?filter=kind=component",
>     r => { let b=""; r.on("data",d=>b+=d); r.on("end",()=>{
>       for (const e of JSON.parse(b))
>         console.log(e.metadata.name, e.metadata.annotations?.["github.com/project-slug"]); }); });'
> ```

## 3. Enable the integration

> Required for **GitHub Enterprise Server** (renders the `integrations.github` block so the
> card can read `apiBaseUrl`) and to inject the optional backend token (sections 4–5). For
> the public github.com card, OAuth (section 1) + the annotation (section 2) are enough and
> this step is optional.

### Public GitHub (github.com)

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --namespace openchoreo-control-plane \
  --set backstage.externalCI.githubActions.enabled=true \
  --set backstage.externalCI.githubActions.oauth.clientId="<your-oauth-client-id>" \
  --reset-then-reuse-values
```

The defaults (`host: github.com`, empty `apiBaseUrl`) are sufficient.

### GitHub Enterprise Server

```bash
helm upgrade --install openchoreo-control-plane install/helm/openchoreo-control-plane \
  --namespace openchoreo-control-plane \
  --set backstage.externalCI.githubActions.enabled=true \
  --set backstage.externalCI.githubActions.host=ghe.example.com \
  --set backstage.externalCI.githubActions.apiBaseUrl=https://ghe.example.com/api/v3 \
  --set backstage.externalCI.githubActions.oauth.clientId="<your-oauth-client-id>" \
  --reset-then-reuse-values
```

## 4. (Optional) Provision a GitHub token — backend features only

> **Optional — skip sections 4–5 if you only want the GitHub Actions card.** This token
> powers the Backstage **backend** GitHub integration (catalog ingestion, scaffolder git
> operations, TechDocs). It is **not** the credential the GitHub Actions card uses to read
> `workflow_run` data — that path is per-user OAuth
> ([section 1](#1-enable-per-user-github-sign-in-required-for-the-actions-card)). The card
> works without it; provision this token only if you also rely on those backend features.

The Backstage GitHub integration needs an API token with read access to the repositories
whose workflow_runs you want to surface.

- **For github.com, lightweight setup:** create a [fine-grained personal access token](https://github.com/settings/personal-access-tokens/new)
  scoped to the relevant repositories with `Actions: Read` and `Metadata: Read` permissions.
- **For org-wide / production setup:** create a [GitHub App](https://docs.github.com/en/apps/creating-github-apps),
  install it on the org, and pass the installation token. App tokens scale better and don't
  expire with the user that created them.
- **For GitHub Enterprise Server:** create the token on your GHES instance, then set both
  `host` and `apiBaseUrl` in the Helm values.

## 5. (Optional) Store the token

> Only needed if you provisioned the token in section 4. Skip if you only want the
> Actions card.

Seed it into the secret backend (the convention is `secret/backstage-<secretKey>`, value
under the `value` property):

```bash
kubectl exec -n openbao openbao-0 -- sh -c '
  export BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN=root
  bao kv put secret/backstage-github-actions-token value="<your-github-token>"
'
```

The `backstage-secrets` ExternalSecret already maps `github-actions-token` →
`backstage-github-actions-token`, so force a re-sync to pull the new value:

```bash
kubectl annotate externalsecret backstage-secrets -n openchoreo-control-plane \
  force-sync=$(date +%s) --overwrite
```

**For installs that create `backstage-secrets` directly** (no ExternalSecret), add the key
to that Secret instead. Export the token first so the command is copy-paste-safe:

```bash
export GITHUB_TOKEN="<your-github-token>"

kubectl -n openchoreo-control-plane patch secret backstage-secrets \
  --type='json' \
  -p='[{"op":"add","path":"/data/github-actions-token","value":"'"$(printf %s "$GITHUB_TOKEN" | base64 | tr -d '\n')"'"}]'
```

The Secret name is taken from `.Values.backstage.secretName`. The key must literally be
`github-actions-token`.

## What ships in this release

| Surface                                | Status                                    |
| -------------------------------------- | ----------------------------------------- |
| Helm value `externalCI.githubActions`  | ✅ in this release                         |
| `GITHUB_HOST`, `GITHUB_API_BASE_URL`, `GITHUB_TOKEN` env vars injected into Backstage | ✅ in this release                         |
| `app-config.ci.yaml` `integrations.github` block | ✅ in this release                         |
| `auth.providers.github` OAuth provider for the Actions card | ✅ in this release                         |
| Component-creation wizard option       | 🚧 tracked in `openchoreo/backstage-plugins` |
| Reusable workflow → Workload bridge    | 🚧 deferred to PR-B of #3551               |
| GitHub OIDC token validation           | 🚧 deferred to PR-B of #3551               |

## Troubleshooting

**The card shows a sign-in / "Log in" prompt that fails, or `/api/auth/github/start`
returns `404 No auth provider registered for 'github'`.**
The GitHub OAuth provider is not registered. Complete [section 1](#1-enable-per-user-github-sign-in-required-for-the-actions-card):
create the OAuth App, add the `auth.providers.github.<env>` block, and restart Backstage.
The provider env key must match `auth.environment` (chart default `development`).

**The GitHub login popup opens but errors with a `redirect_uri` mismatch.**
The OAuth App's Authorization callback URL must be exactly
`<portal-base-url>/api/auth/github/handler/frame` (scheme, host, and port all matching the
portal URL the browser uses).

**The "GitHub Actions" tab shows no runs even though the workflow ran.**
First verify the annotation is present on the Backstage **catalog entity** and matches the
repository slug exactly (`<org>/<repo>`, case-sensitive) — see [section 2](#2-annotate-components).
For a **private** repository, the signed-in user must have access to it on GitHub and must
have approved the `repo` scope during the OAuth consent. If you also use the backend GitHub
integration, check the Backstage pod logs for `Bad credentials` or `403`; if you see these,
the `github-actions-token` Secret key is missing or the token lacks `Actions: Read`.

**The Backstage pod fails to start with "secret key not found".**
The `github-actions-token` Secret key is declared `optional: true` in the Deployment so
the pod will still start without it; if you see this error, your cluster may be running
an older chart version. Upgrade with `helm upgrade` after pulling the latest chart.

**GitHub Enterprise Server users see `Cannot find host` errors.**
Both `host` and `apiBaseUrl` must be set. The plugin uses `host` for `git` URLs and
`apiBaseUrl` for API calls; setting only one will cause the other to fall back to
github.com defaults.
