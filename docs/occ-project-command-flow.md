# `occ project list` / `occ project get` — End-to-End Call Flow

How `occ project list` and `occ project get default` travel from the binary to the
Kubernetes API and back to your terminal. Every box names the **function** and the
**file** that runs.

---

## 1. The whole path at a glance

```mermaid
flowchart TD
    subgraph cli["occ process (client side)"]
        MAIN["main()<br/>cmd/occ/main.go"]
        BUILD["BuildRootCmd()<br/>internal/occ/root/root.go"]
        PRE["PersistentPreRunE<br/>config.EnsureContext / ApplyContextDefaults<br/>internal/occ/cmd/config/config.go"]
        PRERUN["PreRunE: auth.RequireLogin()<br/>internal/occ/auth/require_login.go"]
        RUNE["RunE closure<br/>internal/occ/cmd/project/cmd.go"]
        NEW["New(cl)<br/>project.New()<br/>internal/occ/cmd/project/project.go"]
        OPC["(*Project).List() / .Get()<br/>internal/occ/cmd/project/project.go"]
        NEWCLI["client.NewClient()<br/>internal/occ/resources/client/openapi_client.go"]
        GEN["gen.ClientWithResponses<br/>ListProjectsWithResponse()<br/>internal/openchoreo-api/api/gen/client.gen.go"]
        OUT["printList() / yaml.Marshal + fmt.Print<br/>internal/occ/cmd/project/project.go"]
    end

    subgraph srv["openchoreo-api server"]
        HANDLER["(*Handler).ListProjects() / .GetProject()<br/>internal/openchoreo-api/api/handlers/projects.go"]
        AUTHZ["projectServiceWithAuthz<br/>internal/openchoreo-api/services/project/service_authz.go"]
        SVC["projectService.ListProjects() / .GetProject()<br/>internal/openchoreo-api/services/project/service.go"]
    end

    K8S[("Kubernetes API<br/>Project CRD<br/>namespace: default")]

    MAIN --> BUILD --> PRE --> PRERUN --> RUNE --> NEW --> OPC
    OPC --> NEWCLI --> GEN
    GEN == "HTTP + Bearer token" ==> HANDLER
    HANDLER --> AUTHZ --> SVC --> K8S
    K8S --> SVC --> GEN == "HTTP 200 JSON" ==> OUT
```

---

## 2. Phase 1 — Process start and context bootstrap

`main()` in `cmd/occ/main.go:17` builds the command tree, then cobra walks it:

```mermaid
sequenceDiagram
    autonumber
    participant SH as shell
    participant M as main()<br/>cmd/occ/main.go
    participant R as BuildRootCmd()<br/>internal/occ/root/root.go
    participant P as project.NewProjectCmd()<br/>internal/occ/cmd/project/cmd.go
    participant C as config.EnsureContext()<br/>config.go:392
    participant D as config.ApplyContextDefaults()<br/>config.go:298
    participant CFG as ~/.openchoreo/config

    SH->>M: occ project list
    M->>R: BuildRootCmd()
    R->>P: project.NewProjectCmd(f)
    P-->>R: cobra.Command "project" + list/get/delete/deploy/scaffold
    R-->>M: rootCmd
    Note over M: rootCmd.SetArgs(lint.NormalizeArgs(os.Args[1:]))
    M->>C: PersistentPreRunE
    C->>CFG: IsConfigFileExists()
    alt no config file yet
        C->>CFG: create default context (namespace=default)
    end
    C->>D: ApplyContextDefaults(cmd)
    D->>CFG: LoadStoredConfig()
    D-->>D: applyIfNotSet(cmd, "namespace", curCtx.Namespace)
    D-->>M: nil
```

Key files:

| Function | File | Purpose |
|---|---|---|
| `main()` | `cmd/occ/main.go:17` | entrypoint, sets `PersistentPreRunE` |
| `BuildRootCmd()` | `internal/occ/root/root.go:56` | registers every subcommand on the root `occ` |
| `config.EnsureContext()` | `internal/occ/cmd/config/config.go:392` | creates `~/.openchoreo/config` + default context if absent |
| `config.ApplyContextDefaults()` | `internal/occ/cmd/config/config.go:298` | fills `--namespace` from the current context when the flag was not typed |
| `config.LoadStoredConfig()` | `internal/occ/cmd/config/storage.go:44` | reads and YAML-unmarshals `~/.openchoreo/config` |

This is why `occ project list` with no `--namespace` still queried `default`: the
namespace comes from the stored context, not from your command line.

---

## 3. Phase 2 — `occ project list`

```mermaid
flowchart TD
    A["occ project list"] --> B["cobra resolves 'project list'<br/>internal/occ/cmd/project/cmd.go:32<br/>newListCmd()"]
    B --> C["PreRunE = auth.RequireLogin()<br/>internal/occ/auth/require_login.go:31"]
    C --> C1["auth.IsLoggedIn()<br/>require_login.go:42"]
    C1 --> C2{"security disabled<br/>on control plane?"}
    C2 -- yes --> D
    C2 -- no --> C3{"token present<br/>and unexpired?"}
    C3 -- no --> C4["auth.RefreshToken()<br/>internal/occ/auth/token.go:90"]
    C3 -- yes --> D
    C4 --> D["RunE: cl, _ := f()"]
    D --> E["f() = client.NewClient()<br/>openapi_client.go:47"]
    E --> E1["config.GetCurrentControlPlane()<br/>config.go:647"]
    E --> E2["config.GetCurrentCredential()<br/>config.go:610"]
    E --> E3["gen.NewClientWithResponses(url,<br/>30s timeout, request editor)"]
    E3 --> E4["request editor injects<br/>'Authorization: Bearer &lt;token&gt;'<br/>and refreshes if expired"]
    D --> F["project.New(cl).List(ListParams{Namespace: flags.GetNamespace(cmd)})<br/>project.go:27, project.go:32"]
    F --> G["cmdutil.RequireFields('list','project', {namespace})<br/>internal/occ/cmdutil/require.go:15"]
    G --> H["pagination.FetchAll(closure)<br/>internal/occ/cmd/pagination/pagination.go:18"]
    H -->|loop, chunk=500| I["client.ListProjects(ctx, ns, params)<br/>openapi_client.go:121"]
    I --> J["ListProjectsWithResponse(ctx, ns, params)<br/>client.gen.go:23548<br/>GET /api/v1/namespaces/{ns}/projects?limit=500"]
    J --> K{"resp.JSON200 != nil?"}
    K -- no --> K1["apiError(status, body)<br/>openapi_client.go:17"]
    K -- yes --> L["append items, follow<br/>result.Pagination.NextCursor"]
    L -->|more pages| H
    L -->|next == ''| M["printList(items)<br/>project.go:100"]
    M --> N["tabwriter NAME / TYPE / AGE"]
    N --> O["projectType(proj) = spec.type.Kind/Name<br/>project.go:123"]
    N --> P["utils.FormatAge(metadata.creationTimestamp)<br/>internal/occ/cmd/utils/utils.go:12"]
    O & P --> Q["w.Flush() → stdout"]

    classDef fail fill:#ffe0e0,stroke:#c00;
    class K1 fail;
```

Output shape:

```
NAME      TYPE                         AGE
default   ClusterProjectType/default   27d
```

- `AGE` comes from `utils.FormatAge()` — `s` / `m` / `h` / `d` buckets off
  `metadata.creationTimestamp`, hence `27d` from `2026-09-03T14:18:11Z`.
- `TYPE` is `spec.type.kind/spec.type.name`; when `kind` is nil `projectType()`
  substitutes `ProjectType` to match the API default.

---

## 4. Phase 3 — `occ project get default`

```mermaid
flowchart TD
    A["occ project get default"] --> B["cobra resolves 'project get'<br/>internal/occ/cmd/project/cmd.go:54<br/>newGetCmd()"]
    B --> B1["Args = cmdutil.ExactOneArgWithUsage()<br/>internal/occ/cmdutil/args.go<br/>(exit 2 on wrong arity)"]
    B1 --> C["PreRunE = auth.RequireLogin()"]
    C --> D["RunE: cl, _ := f() → client.NewClient()"]
    D --> E["project.New(cl).Get(GetParams{Namespace, ProjectName})<br/>project.go:27, project.go:63"]
    E --> F["cmdutil.RequireFields('get','project',{namespace})"]
    F --> G["client.GetProject(ctx, ns, 'default')<br/>openapi_client.go:526"]
    G --> H["GetProjectWithResponse(ctx, ns, 'default')<br/>client.gen.go:23583<br/>GET /api/v1/namespaces/default/projects/default"]
    H --> I{"resp.JSON200 != nil?"}
    I -- no --> I1["apiError(status, body)"]
    I -- yes --> J["sigs.k8s.io/yaml.Marshal(result)"]
    J --> K["fmt.Print(string(data)) → stdout"]

    classDef fail fill:#ffe0e0,stroke:#c00;
    class I1 fail;
```

Difference from `list`: **no pagination loop** and **no `printList`** — the whole
`gen.Project` object is marshalled straight to YAML, which is why the output shows
the full `metadata`, `spec` and `status` blocks, including
`status.latestRelease.name: default-6d675ddbf6` and the `Created` / `Ready`
conditions the controller wrote.

---

## 5. Server side — what answers the HTTP call

```mermaid
sequenceDiagram
    autonumber
    participant C as occ client
    participant MW as middleware/authn+audit
    participant H as (*Handler).ListProjects<br/>handlers/projects.go:19
    participant A as projectServiceWithAuthz<br/>service_authz.go:62
    participant F as services.FilteredList
    participant S as projectService.ListProjects<br/>service.go:137
    participant K as controller-runtime client
    participant CR as Project CRD

    C->>MW: GET /api/v1/namespaces/default/projects?limit=500
    MW->>H: ListProjectsRequestObject{NamespaceName}
    H->>H: NormalizeListOptions(limit, cursor, labelSelector)
    H->>A: ListProjects(ctx, ns, opts)
    A->>F: FilteredList(pageOpts, authz.Check(ViewProject))
    loop per page
        F->>S: ListProjects(ctx, ns, pageOpts)
        S->>K: List(ProjectList, InNamespace(ns), limit, continue)
        K->>CR: LIST project
        CR-->>K: items + continue token
        K-->>F: items
        F->>F: drop projects failing authz.ActionViewProject
    end
    F-->>A: authorized items + nextCursor
    A-->>H: ListResult[openchoreov1alpha1.Project]
    H->>H: convertList[CRD, gen.Project]
    H-->>C: 200 {items[], pagination{nextCursor}}
```

For `get`, `projectServiceWithAuthz.GetProject()` (`service_authz.go:78`) does a
single `authz.Check(ViewProject)` instead of the per-page filter, then calls
`projectService.GetProject()` (`service.go:169`), which is a plain
`k8sClient.Get(ctx, ObjectKey{Name, Namespace}, project)`.

`audit/exemptions.go:117` marks `ListProjects` as `reasonRead`, which is why reads
are not written to the audit trail the way writes are.

---

## 6. Full call table

| # | Function | File | Role |
|---|---|---|---|
| 1 | `main()` | `cmd/occ/main.go:17` | builds root, installs `PersistentPreRunE`, handles exit codes |
| 2 | `BuildRootCmd()` | `internal/occ/root/root.go:56` | wires `project.NewProjectCmd(f)` and every other subcommand |
| 3 | `config.EnsureContext()` | `internal/occ/cmd/config/config.go:392` | default context bootstrap |
| 4 | `config.ApplyContextDefaults()` | `internal/occ/cmd/config/config.go:298` | `--namespace` default from stored context |
| 5 | `config.LoadStoredConfig()` | `internal/occ/cmd/config/storage.go:44` | reads `~/.openchoreo/config` |
| 6 | `project.NewProjectCmd()` | `internal/occ/cmd/project/cmd.go:15` | parent `project` command |
| 7 | `project.newListCmd()` | `internal/occ/cmd/project/cmd.go:32` | `occ project list` |
| 8 | `project.newGetCmd()` | `internal/occ/cmd/project/cmd.go:54` | `occ project get` |
| 9 | `auth.RequireLogin()` | `internal/occ/auth/require_login.go:31` | `PreRunE` login gate |
| 10 | `auth.IsLoggedIn()` | `internal/occ/auth/require_login.go:42` | security-disabled short-circuit, token check |
| 11 | `auth.RefreshToken()` | `internal/occ/auth/token.go:90` | renew an expired token |
| 12 | `flags.GetNamespace()` | `internal/occ/flags/flags.go:22` | read the `--namespace` flag |
| 13 | `project.New()` | `internal/occ/cmd/project/project.go:27` | wraps `client.Interface` into `*Project` |
| 14 | `(*Project).List()` | `internal/occ/cmd/project/project.go:32` | list orchestration + pagination |
| 15 | `(*Project).Get()` | `internal/occ/cmd/project/project.go:63` | single fetch + YAML print |
| 16 | `cmdutil.RequireFields()` | `internal/occ/cmdutil/require.go:15` | required-parameter guard |
| 17 | `cmdutil.ExactOneArgWithUsage()` | `internal/occ/cmdutil/args.go` | arity guard for `get` |
| 18 | `pagination.FetchAll()` | `internal/occ/cmd/pagination/pagination.go:18` | cursor loop, chunk 500 |
| 19 | `client.NewClient()` | `internal/occ/resources/client/openapi_client.go:47` | builds generated client, bearer token, 30s timeout |
| 20 | `apiError()` | `internal/occ/resources/client/openapi_client.go:17` | non-200 → readable error |
| 21 | `(*Client).ListProjects()` | `internal/occ/resources/client/openapi_client.go:121` | list wrapper |
| 22 | `(*Client).GetProject()` | `internal/occ/resources/client/openapi_client.go:526` | get wrapper |
| 23 | `ListProjectsWithResponse()` | `internal/openchoreo-api/api/gen/client.gen.go:23548` | builds/sends the HTTP request |
| 24 | `GetProjectWithResponse()` | `internal/openchoreo-api/api/gen/client.gen.go:23583` | builds/sends the HTTP request |
| 25 | `printList()` | `internal/occ/cmd/project/project.go:100` | tabwriter table |
| 26 | `projectType()` | `internal/occ/cmd/project/project.go:123` | `Kind/Name` column |
| 27 | `utils.FormatAge()` | `internal/occ/cmd/utils/utils.go:12` | `AGE` column |
| 28 | `(*Handler).ListProjects()` | `internal/openchoreo-api/api/handlers/projects.go:19` | server handler |
| 29 | `(*Handler).GetProject()` | `internal/openchoreo-api/api/handlers/projects.go:96` | server handler |
| 30 | `projectServiceWithAuthz.ListProjects()` | `internal/openchoreo-api/services/project/service_authz.go:62` | per-item `ActionViewProject` filter |
| 31 | `projectServiceWithAuthz.GetProject()` | `internal/openchoreo-api/services/project/service_authz.go:78` | single `ActionViewProject` check |
| 32 | `projectService.ListProjects()` | `internal/openchoreo-api/services/project/service.go:137` | controller-runtime `List` |
| 33 | `projectService.GetProject()` | `internal/openchoreo-api/services/project/service.go:169` | controller-runtime `Get` |

---

## 7. The three-layer rule

Every `occ` subcommand follows the same shape, so once you have read one you have
read all of them:

```mermaid
flowchart LR
    A["layer 1 — cobra wiring<br/>cmd.go: flags, PreRunE, RunE"] --> B["layer 2 — resource logic<br/>project.go: params, pagination, printing"]
    B --> C["layer 3 — transport<br/>openapi_client.go: token, HTTP, status codes"]
    C --> D["layer 4 — generated client<br/>client.gen.go: URL + JSON"]
    D --> E["layer 5 — API server<br/>handlers → services → k8s"]
```

- **Layer 1** never touches HTTP; it only builds `Params` structs.
- **Layer 2** is per-resource and is where output formatting lives.
- **Layer 3** is shared by every resource — one place for auth and error mapping.
- **Layer 4** is generated from `openapi/`; do not hand-edit it.
- **Layer 5** is the server, and is a separate deployment from `occ` itself.

`occ lint` is the exception that proves the rule: it skips layers 3–5 entirely,
which is why `internal/occ/cmd/project/cmd.go` imports `client` but
`internal/occ/cmd/lint/` does not.
