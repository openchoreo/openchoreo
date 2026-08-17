# PATCHES — idp-openchoreo

Registro de **todo lo que este espejo difiere del upstream**. Si un archivo del upstream cambió y no
está en la tabla, es un bug: o falta la fila, o el cambio no debió existir.

| | |
|---|---|
| Upstream | [`openchoreo/openchoreo`](https://github.com/openchoreo/openchoreo) |
| Tag fijado | **`v1.2.2`** |
| Commit | `58758d662fa07bf885acc22045c6919f4aa2e8c1` |
| Diff objetivo | **2 parches de seguridad** + higiene de fork |

## Reglas

1. Antes de escribir un parche, agotá el orden: **(a)** un CR o un value de Helm en `idp-platform`;
   **(b)** un archivo **nuevo**; **(c)** recién ahí, editar un archivo del upstream.
2. Un parche = **un commit** con prefijo `PATCH:` en el subject.
3. Todo parche que toque código del upstream lleva un **PR upstream en paralelo**. Sin PR upstream, el
   parche es deuda permanente.
4. Toda fila necesita **condición de retiro**. "Nunca" es una respuesta válida sólo para higiene de
   fork; para código, no.
5. **Nunca** reformatear, renombrar ni reorganizar archivos del upstream.
6. Al rebasear a un tag nuevo, revisá esta tabla archivo por archivo: si upstream tocó un archivo
   parcheado, resolvé a mano y dejá constancia.

## Parches vigentes

| # | Parche | Archivos | Motivo | Issue / PR upstream | Condición de retiro |
|---|---|---|---|---|---|
| 0 | Higiene de fork | `.github/CODEOWNERS` | El CODEOWNERS del upstream apunta a equipos `@openchoreo/*` que no existen en `fondomp-production`; GitHub lo marca inválido y no asigna revisores. `.github/` tiene precedencia sobre la raíz, así que no alcanza con agregar uno nuevo | — (no aplica: es governance del fork, no código) | Permanente mientras exista el fork |

## Parches planificados

Reservados por el plan de agentes. **No implementados todavía** — no borres la fila, completala.

| # | Parche | Archivos previstos | Motivo | Track |
|---|---|---|---|---|
| 1 | **CRITICAL** · denylist del proxy del `cluster-gateway` | `internal/cluster-gateway/validator.go` (+ su test) | Tres defectos: `/apis/v1/serviceaccounts` **nunca matchea** (el core API va bajo `/api/v1`, no `/apis/<group>/<version>`); el commit `28e0338` **eliminó** `/api/v1/secrets` en un PR no relacionado; y el subrecurso **TokenRequest** (`POST …/serviceaccounts/{name}/token`) no está contemplado — emite un JWT vivo para cualquier ServiceAccount. El test hoy valida el string equivocado y por eso pasa | T11 (issue upstream #4237) |
| 2 | **CRITICAL** · RBAC del `cluster-agent` | ClusterRole en los 3 charts (data, observability, workflow plane) | `clusterroles`/`clusterrolebindings` con verbo `*` → auto-escalación de privilegios, **cluster-admin de facto**: el comodín elude la prevención que exige `escalate`/`bind` | T12 |
| 3 | **HIGH** · `POST /rca-agent/analyze` sin authn | `agents/sre-agent/src/api/agent_routes.py` | Endpoint sin autenticación | T13 |
| 4 | **HIGH** · bootstrap de authz sin `--prune` | Chart del control plane | Sacar un binding de `values.yaml` **no revoca el acceso**. Requiere `--prune` con label selector y etiquetar los CRs generados | T13 |
| 5 | *(reservado)* resolver de JWT para Auth0 | — | Presupuestado en D2 por si el control plane rechaza el token por algo imprevisto. **Puede no hacer falta**: el resolver es configurable | T06 / D2 |

## Parches retirados

| # | Parche | Retirado en | Motivo |
|---|---|---|---|
| _(ninguno todavía)_ | | | |
