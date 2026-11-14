# `occ apply` — End-to-End Call Flow

How `occ apply -f manifest.yaml` turns a YAML file into create/update calls against
the OpenChoreo API. Every box names the **function** and the **file** that runs.

Unlike `occ project list` / `occ project get`, `apply` is **kind-driven**: it reads
`kind` out of your YAML, looks that kind up in a registry table, and dispatches
through function pointers stored in that table. There is no `if kind == "Project"`
chain anywhere in the codebase.

---

## 1. The whole path at a glance

```mermaid
flowchart TD
    subgraph cli["occ process (client side)"]
        MAIN["main()<br/>cmd/occ/main.go"]
        BUILD["BuildRootCmd()<br/>internal/occ/root/root.go"]
        PRERUN["PreRunE: auth.RequireLogin()<br/>internal/occ/auth/require_login.go"]
        RUNE["RunE: reads --file<br/>internal/occ/cmd/apply/cmd.go"]
        APPLY["Apply(cl, Params)<br/>internal/occ/cmd/apply/apply.go:34"]
        DISC["discoverResourceFiles()<br/>apply.go:241"]
        READ["readResourceContent()<br/>apply.go:279"]
        PARSE["parseYAMLResources()<br/>apply.go:308"]
        ONE["applyResource()<br/>apply.go:130"]
        REG["getResourceRegistry()<br/>internal/occ/cmd/apply/registry.go:54"]
        STRIP["stripKindAndAPIVersion()<br/>apply.go:123"]
        DISP["entry.get / entry.create / entry.update<br/>registry.go — func pointers"]
    end

    subgraph srv["openchoreo-api server"]
        HANDLER["ProjectHandler.CreateProject / UpdateProject<br/>internal/openchoreo-api/api/handlers/projects.go"]
        AUTHZ["projectServiceWithAuthz<br/>services/project/service_authz.go"]
        SVC["projectService.CreateProject / UpdateProject<br/>services/project/service.go"]
    end

    K8S[("Kubernetes API<br/>Project CRD")]

    MAIN --> BUILD --> PRERUN --> RUNE --> APPLY
    APPLY --> DISC --> READ --> PARSE --> ONE
    APPLY -.->|builds once| REG
    REG -.->|registry table| DISP
    ONE --> STRIP --> DISP
    DISP == "HTTP + Bearer token" ==> HANDLER
    HANDLER --> AUTHZ --> SVC --> K8S
```

---

## 2. Phase 1 — Command wiring

```mermaid
sequenceDiagram
    autonumber
    participant SH as shell
    participant M as main()<br/>cmd/occ/main.go
    participant R as BuildRootCmd()<br/>internal/occ/root/root.go
    participant AC as apply.NewApplyCmd()<br/>cmd/apply/cmd.go:13
    participant AU as auth.RequireLogin()<br/>auth/require_login.go:31
    participant NC as client.NewClient()<br/>openapi_client.go:47
    participant AP as apply.Apply()<br/>apply.go:34

    SH->>M: occ apply -f manifests/app.yaml
    M->>R: BuildRootCmd()
    R->>AC: apply.NewApplyCmd(f)
    AC->>AC: register -f/--file string flag
    AC-->>R: cobra.Command "apply"
    R-->>M: rootCmd
    Note over M: rootCmd.Execute()
    M->>AU: PersistentPreRunE → EnsureContext / ApplyContextDefaults
    M->>AU: PreRunE → RequireLogin()
    AU->>AU: IsLoggedIn() / RefreshToken()
    AU->>NC: RunE → f()
    NC-->>M: *client.Client (30s timeout, bearer editor)
    M->>AP: Apply(cl.(*client.Client), Params{FilePath})
    Note over M: type assertion cl.(*client.Client)<br/>bypasses the client.Interface wrapper
```

| Function | File | Role |
|---|---|---|
| `main()` | `cmd/occ/main.go:17` | builds root, installs `PersistentPreRunE` |
| `BuildRootCmd()` | `internal/occ/root/root.go:56` | registers `apply.NewApplyCmd(f)` |
| `apply.NewApplyCmd()` | `internal/occ/cmd/apply/cmd.go:13` | defines `apply`, `-f/--file`, `PreRunE`, `RunE` |
| `auth.RequireLogin()` | `internal/occ/auth/require_login.go:31` | login gate |
| `client.NewClient()` | `internal/occ/resources/client/openapi_client.go:47` | builds the generated client + bearer token |
| `(*Client).GetClient()` | `internal/occ/resources/client/openapi_client.go:92` | unwraps to `*gen.ClientWithResponses` |

Note the one wrinkle in this command: `RunE` does `cl.(*client.Client)` — a
concrete type assertion on the `client.Interface` value. `apply` needs the raw
`*gen.ClientWithResponses` because the registry closures call generated methods
directly, so the interface abstraction is deliberately unwrapped here. This is the
only `occ` subcommand that does this.

---

## 3. Phase 2 — Discover, read, parse

```mermaid
flowchart TD
    A["Apply(cl, Params)<br/>apply.go:34"] --> A1{"params.FilePath == ''?"}
    A1 -- yes --> A2["error: file path is required<br/>exit 1"]
    A1 -- no --> B["genClient := c.GetClient()<br/>apply.go:39"]
    B --> C["discoverResourceFiles(path)<br/>apply.go:241"]
    C --> C1{"path starts with<br/>http:// or https://?"}
    C1 -- yes --> C2["return path as-is<br/>(single remote file)"]
    C1 -- no --> C3{"os.Stat(path)"}
    C3 -->|not exist| C4["error: path does not exist"]
    C3 -->|is a file| C5["return [path]"]
    C3 -->|is a dir| C6["filepath.Walk — collect<br>*.yaml and *.yml recursively<br/>apply.go:259"]
    C2 & C5 & C6 --> D{"len(files) == 0?"}
    D -- yes --> D1["error: no YAML files found in: path"]
    D -- no --> E["registry := getResourceRegistry()<br/>registry.go:54<br/>36 kinds registered"]
    E --> F["defaultNamespace := resolveDefaultNamespace()<br/>apply.go:232<br/>config.GetCurrentContext()"]
    F --> G["ctx := context.Background()<br/>for each file:"]

    G --> G1["readResourceContent(ctx, file)<br/>apply.go:279"]
    G1 --> G1a{"http:// or https://?"}
    G1a -- yes --> G1b["http.Get + status must be 200<br/>apply.go:280"]
    G1a -- no --> G1c["os.ReadFile<br/>apply.go:297"]
    G1b --> H
    G1c --> H
    H["parseYAMLResources(content)<br/>apply.go:308"] --> H1["yaml.NewDecoder loop<br/>multi-document support"]
    H1 --> H2{"doc == nil or doc['kind'] == nil?"}
    H2 -- yes --> H3["skip this document"]
    H2 -- no --> H4["append to resources[]"]
    H4 --> H1
    H3 --> H1
    H1 -->|"io.EOF"| I["for each resource: applyResource(...)"]

    classDef fail fill:#ffe0e0,stroke:#c00;
    class A2,C4,D1 fail;
```

Important behaviours here:

- **`apply` is not recursive-by-design but is recursive-by-necessity.** Given a
  file it processes one file; given a directory it `filepath.Walk`s and collects
  every `.yaml`/`.yml` (apply.go:266-268). There is no `-b` / `-detect` flag the way
  `occ lint vali` has — the directory *is* the flag.
- **Remote URLs are first-class.** A `http://` / `https://` path bypasses
  `os.Stat` entirely and is downloaded with `http.DefaultClient` (apply.go:280).
  The `#nosec G107` annotation is there because this is intentional.
- **Documents without `kind` are skipped silently** (apply.go:320) — a `ConfigMap`
  mixed into a multi-doc YAML file will not be applied and will not be reported.
- **Errors accumulate, they do not abort.** A failed read or parse is appended to
  `errs` and the loop `continue`s to the next file (apply.go:62, 68).

---

## 4. Phase 3 — `applyResource`: the decision ladder

This is the heart of the command. `applyResource()` runs seven guard clauses in
order, then does a get → create-or-update.

```mermaid
flowchart TD
    A["applyResource(ctx, genClient, registry, resource, defaultNamespace)<br/>apply.go:130"] --> B["extractResourceInfo(resource)<br/>apply.go:97"]
    B --> B1{"kind == ''?"}
    B1 -- yes --> X1["ERR: resource is missing 'kind'"]
    B1 -- no --> B2{"metadata.name == ''?"}
    B2 -- yes --> X2["ERR: &lt;Kind&gt;: resource is missing 'metadata.name'"]
    B2 -- no --> C["info = {kind, apiVersion, name, namespace}"]

    C --> D{"readOnlyKinds[kind]?<br/>registry.go:35 — only 'RenderedRelease'"}
    D -- yes --> X3["ERR: kind is read-only, not supported by apply"]
    D -- no --> E{"apiVersion set and<br/>does NOT contain<br/>'openchoreo.dev'?"}
    E -- yes --> X4["ERR: unsupported apiVersion"]
    E -- no --> F{"registry[kind] exists?<br/>36 kinds"}
    F -- no --> X5["ERR: unsupported kind<br/>(lists all supported kinds)"]
    F -- yes --> G["entry := registry[kind]<br/>scope + capability + 3 funcs"]

    G --> H{"entry.scope == scopeNamespaced?"}
    H -- yes --> H1{"metadata.namespace set?"}
    H1 -- yes --> H2["ns = metadata.namespace"]
    H1 -- no --> H3["ns = defaultNamespace<br/>from occ config context"]
    H2 & H3 --> H4{"ns still empty?"}
    H4 -- yes --> X6["ERR: namespace is required<br/>(YAML or 'occ config set-context')"]
    H4 -- no --> I
    H -- no --> I["stripKindAndAPIVersion()<br/>delete kind + apiVersion<br/>json.Marshal → body"]

    I --> J["entry.get(ctx, c, ns, name)<br/>returns HTTP status code only"]
    J --> K{"status code?"}

    K -->|"200 OK<br/>exists"| L{"entry.capability<br/>== capCreateOnly?"}
    L -- yes --> X7["ERR: already exists and cannot be updated<br/>(create-only resource)"]
    L -- no --> M["entry.update(ctx, c, ns, name, body)"]
    M --> M1{"code == 200?"}
    M1 -- no --> X8["ERR: update failed: parseErrorBody()"]
    M1 -- yes --> N["print '&lt;kind&gt;/&lt;name&gt; configured'"]

    K -->|"404 NotFound<br/>absent"| O["entry.create(ctx, c, ns, body)"]
    O --> O1{"code == 200 or 201?"}
    O1 -- no --> X9["ERR: create failed: parseErrorBody()"]
    O1 -- yes --> P["print '&lt;kind&gt;/&lt;name&gt; created'"]

    K -->|"anything else"| X10["ERR: unexpected status when checking existence"]

    N --> Q([return nil])
    P --> Q
    X1 & X2 & X3 & X4 & X5 & X6 & X7 & X8 & X9 & X10 --> Y([return err])

    classDef fail fill:#ffe0e0,stroke:#c00;
    class X1,X2,X3,X4,X5,X6,X7,X8,X9,X10 fail;
    classDef ok fill:#e0ffe0,stroke:#0a0;
    class N,P ok;
```

### The get → create-or-update trick

`apply` has no `GET`-then-diff. It asks the existence question by making the `GET`
call and reading only its **status code**, discarding the body:

```mermaid
sequenceDiagram
    autonumber
    participant AR as applyResource()<br/>apply.go:130
    participant E as entry.get
    participant G as gen.ClientWithResponses
    participant C as entry.create / entry.update
    participant S as openchoreo-api
    participant K as Kubernetes API

    AR->>E: get(ctx, c, ns, name)
    E->>G: GetProjectWithResponse(ctx, ns, name)
    G->>S: GET /api/v1/namespaces/{ns}/projects/{name}
    S-->>G: 404 Not Found
    G-->>E: r.StatusCode() == 404
    E-->>AR: (404, nil)
    Note over AR: switch statusCode<br/>case 404 → create branch
    AR->>C: create(ctx, c, ns, body)
    C->>G: CreateProjectWithBodyWithResponse(...)
    G->>S: POST /api/v1/namespaces/{ns}/projects
    S->>K: create Project
    K-->>S: created
    S-->>G: 201 Created
    G-->>C: (201, body, nil)
    C-->>AR: (201, body, nil)
    AR->>AR: fmt.Printf("project/default created")
```

The registry's `getFn` deliberately returns `(int, error)` rather than the decoded
object — see `registry.go:37`. The `get` closures call `r.StatusCode()` and throw
the body away, because `apply` only needs create-vs-update.

---

## 5. Phase 4 — The registry table

`registry.go` is the extension point. It is a `map[string]resourceEntry` where
each entry holds three function pointers, so `applyResource` never switches on kind.

```mermaid
flowchart LR
    subgraph REG["getResourceRegistry()<br/>registry.go:54"]
        CS["addClusterScopedResources()<br/>registry.go:63<br/>11 kinds"]
        NS["addNamespacedScopedResources()<br/>registry.go:341<br/>25 kinds"]
    end

    ENTRY["resourceEntry<br/>registry.go:47"]
    ENTRY --> SC["scope: scopeCluster<br/>or scopeNamespaced"]
    ENTRY --> CAP["capability: capCreateAndUpdate<br/>or capCreateOnly"]
    ENTRY --> G["getFn<br/>(ctx, c, ns, name) → statusCode"]
    ENTRY --> CR["createFn<br/>(ctx, c, ns, body) → statusCode, body"]
    ENTRY --> UP["updateFn<br/>nil when capCreateOnly<br/>registry.go:52"]

    CS --> ENTRY
    NS --> ENTRY
    G & CR & UP -.->|dispatched by| AR["applyResource()"]
```

### Cluster-scoped kinds (11) — `registry.go:63-340`

`Namespace`, `ClusterComponentType`, `ClusterTrait`, `ClusterWorkflowPlane`,
`ClusterWorkflow`, `ClusterDataPlane`, `ClusterObservabilityPlane`,
`ClusterAuthzRole`, `ClusterAuthzRoleBinding`, `ClusterResourceType`,
`ClusterProjectType`

All three of scope, capability, and the `ns` argument are ignored for these; the
closures take `_` for the namespace parameter.

### Namespaced kinds (25) — `registry.go:341-946`

`Project`, `Component`, `ComponentType`, `Environment`, `DataPlane`,
`WorkflowPlane`, `ObservabilityPlane`, `DeploymentPipeline`, `Trait`,
`SecretReference`, `Workflow`, `Workload`, `ComponentRelease`, `ReleaseBinding`,
`ObservabilityAlertsNotificationChannel`, `AuthzRole`, `AuthzRoleBinding`,
`ResourceType`, `ProjectType`, `Resource`, `ResourceReleaseBinding`,
`ProjectReleaseBinding`, `WorkflowRun`, `ResourceRelease`, `ProjectRelease`

### Create-only kinds (4)

`ComponentRelease` (registry.go:642), `WorkflowRun` (registry.go:888),
`ResourceRelease` (registry.go:907), `ProjectRelease` (registry.go:926)

These declare `capability: capCreateOnly` and have **no `update` closure at all**
(`update` is nil, registry.go:52). Applying one twice gives you
`projectrelease/default-6d675ddbf6: resource already exists and cannot be updated
(create-only resource)` — the `capCreateOnly` check at apply.go:184 fires before the
nil dereference could happen.

### Read-only kinds (1)

`RenderedRelease` (registry.go:35). It is a real CRD with no Create/Update
endpoints, so `apply` rejects it up front at apply.go:143 — before the registry
lookup, because it is not in the registry at all.

### Adding a new kind

```mermaid
flowchart LR
    A["1. add reg['MyKind'] = resourceEntry{...}<br/>to the right add*ScopedResources()"] --> B["2. point get/create/update at the<br/>generated client.gen.go methods"]
    B --> C["3. pick scope + capability"]
    C --> D["4. no changes needed in apply.go<br/>dispatch is table-driven"]
    D --> E["occ apply now understands MyKind"]
```

No edit to `applyResource()`, `Apply()`, or `cmd.go` is required.

---

## 6. Output and exit behaviour

```mermaid
flowchart TD
    A["loop finished"] --> B["applied := totalResources - len(errs)<br/>apply.go:80"]
    B --> C["for each e in errs: print 'Error: e'<br/>apply.go:82"]
    C --> D{"len(errs) > 0?"}
    D -- yes --> E["print 'Applied N resource(s) from M file(s) with K error(s)'<br/>apply.go:87"]
    E --> F["return error 'apply completed with K error(s)'<br/>apply.go:89"]
    F --> G["main() prints 'Error: ...' and os.Exit(1)<br/>cmd/occ/main.go:60"]
    D -- no --> H["print 'Applied N resource(s) from M file(s)'<br/>apply.go:92"]
    H --> I["return nil → exit 0"]

    classDef fail fill:#ffe0e0,stroke:#c00;
    class F,G fail;
    classDef ok fill:#e0ffe0,stroke:#0a0;
    class I ok;
```

Two consequences worth knowing:

- **Partial success is the norm on failure.** `apply` applies everything it can,
  then reports the errors at the end. Exit code 1 does *not* mean nothing was
  applied.
- **`applied` is computed as `totalResources - len(errs)`** (apply.go:80), and
  `errs` holds one entry per failed resource. This arithmetic is correct only
  because every append site corresponds to exactly one resource — worth keeping in
  mind if you add a code path that can append twice.

Example output:

```
namespace/default created
project/default created
deploymentpipeline/default configured

Applied 3 resource(s) from 1 file(s)
```

---

## 7. Full call table

| # | Function | File | Role |
|---|---|---|---|
| 1 | `main()` | `cmd/occ/main.go:17` | builds root, installs `PersistentPreRunE`, maps error → exit 1 |
| 2 | `BuildRootCmd()` | `internal/occ/root/root.go:56` | wires `apply.NewApplyCmd(f)` |
| 3 | `apply.NewApplyCmd()` | `internal/occ/cmd/apply/cmd.go:13` | `apply` command, `-f/--file`, `PreRunE`, `RunE` |
| 4 | `auth.RequireLogin()` | `internal/occ/auth/require_login.go:31` | `PreRunE` login gate |
| 5 | `client.NewClient()` | `internal/occ/resources/client/openapi_client.go:47` | generated client + bearer token, 30s timeout |
| 6 | `(*Client).GetClient()` | `internal/occ/resources/client/openapi_client.go:92` | unwrap to `*gen.ClientWithResponses` |
| 7 | `Apply()` | `internal/occ/cmd/apply/apply.go:34` | top-level orchestration, error accumulation |
| 8 | `discoverResourceFiles()` | `internal/occ/cmd/apply/apply.go:241` | file / dir / URL resolution |
| 9 | `readResourceContent()` | `internal/occ/cmd/apply/apply.go:279` | `os.ReadFile` or HTTP GET |
| 10 | `parseYAMLResources()` | `internal/occ/cmd/apply/apply.go:308` | multi-document YAML → `[]map[string]interface{}` |
| 11 | `resolveDefaultNamespace()` | `internal/occ/cmd/apply/apply.go:232` | namespace from `occ config` context |
| 12 | `config.GetCurrentContext()` | `internal/occ/cmd/config/config.go:586` | reads `~/.openchoreo/config` |
| 13 | `getResourceRegistry()` | `internal/occ/cmd/apply/registry.go:54` | builds the 36-kind table |
| 14 | `addClusterScopedResources()` | `internal/occ/cmd/apply/registry.go:63` | 11 cluster-scoped kinds |
| 15 | `addNamespacedScopedResources()` | `internal/occ/cmd/apply/registry.go:341` | 25 namespaced kinds |
| 16 | `supportedKinds()` | `internal/occ/cmd/apply/registry.go:947` | sorted kind list for the error message |
| 17 | `applyResource()` | `internal/occ/cmd/apply/apply.go:130` | guard ladder + get/create/update |
| 18 | `extractResourceInfo()` | `internal/occ/cmd/apply/apply.go:97` | pulls kind/apiVersion/name/namespace |
| 19 | `stripKindAndAPIVersion()` | `internal/occ/cmd/apply/apply.go:123` | deletes both keys, marshals to JSON |
| 20 | `parseErrorBody()` | `internal/occ/cmd/apply/apply.go:215` | `gen.ErrorResponse` → message, 200-char fallback |
| 21 | `resourceEntry` | `internal/occ/cmd/apply/registry.go:47` | scope + capability + 3 function pointers |
| 22 | `readOnlyKinds` | `internal/occ/cmd/apply/registry.go:35` | `RenderedRelease` rejection set |
| 23 | `(*Handler).CreateProject()` | `internal/openchoreo-api/api/handlers/projects.go:49` | server create handler |
| 24 | `(*Handler).UpdateProject()` | `internal/openchoreo-api/api/handlers/projects.go:124` | server update handler |
| 25 | `projectServiceWithAuthz` | `internal/openchoreo-api/services/project/service_authz.go` | `ActionCreateProject` / `ActionUpdateProject` checks |
| 26 | `projectService` | `internal/openchoreo-api/services/project/service.go` | controller-runtime `Create` / `Update` |

---

## 8. How `apply` differs from the other commands

```mermaid
flowchart TB
    subgraph PROJ["occ project list/get"]
        A1["params struct"] --> A2["pagination.FetchAll()"] --> A3["client.Interface method"] --> A4["server handler"]
    end

    subgraph APPLY["occ apply"]
        B1["-f flag"] --> B2["parseYAMLResources()"] --> B3["registry[kind]<br/>3 func pointers"] --> B4["gen.ClientWithResponses directly"]
    end

    subgraph LINT["occ lint"]
        C1["NormalizeArgs"] --> C2["local file walk"] --> C3["JSON schema validate"] --> C4["stdout report"]
    end

    PROJ & APPLY --> X["HTTP + Bearer token<br/>openchoreo-api"]
    LINT --> Y["no network, no auth"]
```

| Concern | `project list/get` | `apply` | `lint` |
|---|---|---|---|
| Input | flag + positional args | YAML file / directory / URL | YAML file / directory |
| Dispatch | static method call | table of function pointers, keyed by `kind` | schema-driven rules |
| Network | yes | yes | **no** |
| Auth | `RequireLogin()` | `RequireLogin()` | none |
| Multi-doc YAML | n/a | yes (`yaml.Decoder` loop) | yes |
| Partial success | n/a | yes, errors accumulate | n/a |
| Auto-fix | no | no | `-fix` / `-dry-run` |
| Exit codes | 0 / 1 | 0 / 1 | 0 clean, 1 findings, 2 usage |

The three commands share only `main()` and the context bootstrap. `lint` skips the
client, the server and auth entirely, which is why
`config.ShouldSkipContextBootstrap()` exists in `cmd/occ/main.go:32` — without it,
validating a local file would create a `~/.openchoreo/config` on a machine that has
never logged in.
