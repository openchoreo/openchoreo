# PATCHES — idp-openchoreo

Registro de **todo lo que este espejo difiere del upstream**. Si `git diff upstream-main main`
muestra un archivo que no está en alguna de las tablas de abajo, es un bug: o falta la fila, o el
cambio no debió existir.

| | |
|---|---|
| Upstream | [`openchoreo/openchoreo`](https://github.com/openchoreo/openchoreo) |
| Tag fijado | **`v1.2.2`** |
| Commit | `e4a3e0351851c8a980634e9ee91146110bc235aa` |
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
7. **El sha del pin es el del commit, no el del objeto tag.** Los tags del upstream son
   **anotados**, así que `git rev-parse v1.2.2` devuelve el objeto tag
   (`58758d662fa07bf885acc22045c6919f4aa2e8c1`) y no el commit
   (`e4a3e0351851c8a980634e9ee91146110bc235aa`). Usá siempre:

   ```bash
   git rev-parse 'v1.2.2^{commit}'
   ```

   Verificación: `git cat-file -t <tag>` responde `tag` si es anotado, `commit` si es liviano.
   Esta fila existe porque la tabla de arriba tuvo el sha del objeto tag hasta 2026-08-17 (T00b);
   `upstream-main` siempre apuntó al commit correcto, era la documentación la que mentía.

## Parches vigentes

| # | Parche | Archivos | Motivo | Issue / PR upstream | Condición de retiro |
|---|---|---|---|---|---|
| 0 | Higiene de fork · CODEOWNERS | `.github/CODEOWNERS` (**modifica** un archivo del upstream) | El CODEOWNERS del upstream apunta a equipos `@openchoreo/*` que no existen en `fondomp-production`; GitHub lo marca inválido y no asigna revisores. `.github/` tiene precedencia sobre la raíz, así que no alcanza con agregar uno nuevo | — (no aplica: es governance del fork, no código) | Permanente mientras exista el fork |
| 0b | Scaffolding del fork | `.github/README.md`, `PATCHES.md`, `.github/scripts/verify-fork.sh`, `.github/scripts/verify-cluster-agent-rbac.sh`, `.github/scripts/sync-actions-allowlist.sh`, `.github/workflows/idp-upstream-sync.yml`, `.github/workflows/idp-fork-ci.yml`, `idp-sync/` (**archivos nuevos**, el upstream no los tiene) | `.github/README.md` documenta el fork sin tocar el `README.md` del upstream (GitHub le da precedencia para la portada). `PATCHES.md` es este archivo. `verify-fork.sh` es la verificación mínima del espejo — ver abajo **T08b:** se suman la allow-list de Actions y el CI del fork. Actions estaba apagado a nivel repositorio desde T00, lo que tambien apagaba lo nuestro: `idp-upstream-sync.yml` nunca corrio y ningun PR del espejo reportaba checks. `sync-actions-allowlist.sh` enciende Actions y apaga por API cada workflow del upstream, uno por uno; `idp-fork-ci.yml` corre `verify-fork.sh` (+ self-test), `verify-cluster-agent-rbac.sh`, `verify-authz-bootstrap-prune.sh` y `go test` de los paquetes parcheados. | — (governance del fork, no código) | Permanente mientras exista el fork |
| 1 | **CRITICAL** · denylist del proxy del `cluster-gateway` | `internal/cluster-gateway/validator.go`, `internal/cluster-gateway/validator_test.go` | Corrige tres huecos del denylist: `/api/v1/serviceaccounts` cluster-wide (antes estaba escrito como `/apis/v1/serviceaccounts` y nunca matcheaba), `/api/v1/secrets` cluster-wide (regresión de `28e0338`) y `POST /api/v1/namespaces/{namespace}/serviceaccounts/{name}/token` (TokenRequest). El match pasa a ser por borde de segmento, no por substring | [openchoreo/openchoreo#4237](https://github.com/openchoreo/openchoreo/issues/4237), [openchoreo/openchoreo#4511](https://github.com/openchoreo/openchoreo/pull/4511) | Retirar cuando upstream mergee el fix que cierre #4237 y el espejo avance a un tag/release que lo incluya |
| 2 | **CRITICAL** · RBAC del `cluster-agent` | `install/helm/openchoreo-data-plane/templates/cluster-agent/clusterrole.yaml`, `install/helm/openchoreo-data-plane/templates/cluster-agent/role.yaml`, `install/helm/openchoreo-data-plane/templates/cluster-agent/rolebinding.yaml`, `install/helm/openchoreo-observability-plane/templates/cluster-agent/clusterrole.yaml`, `install/helm/openchoreo-observability-plane/templates/cluster-agent/role.yaml`, `install/helm/openchoreo-observability-plane/templates/cluster-agent/rolebinding.yaml`, `install/helm/openchoreo-workflow-plane/templates/cluster-agent/clusterrole.yaml`, `install/helm/openchoreo-workflow-plane/templates/cluster-agent/role.yaml`, `install/helm/openchoreo-workflow-plane/templates/cluster-agent/rolebinding.yaml`, `.github/scripts/verify-cluster-agent-rbac.sh` | El `cluster-agent` proxya requests al API server con su ServiceAccount. `clusterroles`/`clusterrolebindings` con verbo `*` vuelven esa identidad `cluster-admin` de facto y eluden la prevención de escalación basada en `escalate`/`bind`; `secrets` y `serviceaccounts` no necesitan quedar en ClusterRole. El parche saca la **escritura** de `rbac.authorization.k8s.io` de los agentes. **Corregido en T73:** la primera versión movió los permisos namespaced a un `Role` en el namespace del release, pero el agente aplica dentro de **Cells creadas en runtime** (`dp-<ns>-<project>-<env>-<hash>`), que ningún Role fijo puede cubrir: quedaba imposible desplegar nada. Además faltaban `resourcequotas`/`limitranges` (los emite nuestro `ClusterProjectType`) y lectura de RBAC, sin la cual el read-back de `RenderedRelease` aborta y **todo componente reporta NotReady para siempre**. Estado final: workload cluster-wide, RBAC **sólo `get`/`list`/`watch`**, cero verbos de escritura sobre `rbac.authorization.k8s.io` | [openchoreo/openchoreo#4510](https://github.com/openchoreo/openchoreo/pull/4510) | **Retirar cuando [openchoreo/openchoreo#4510](https://github.com/openchoreo/openchoreo/pull/4510) este mergeado EN SU FORMA CORREGIDA y el tag pinneado lo incluya.** La condicion operativa es una sola: `.github/scripts/verify-fork.sh main upstream-main` pasa el **check 6** contra `upstream-main`, o sea que el ClusterRole del upstream ya otorga los kinds de una Cell cluster-wide (incluidos `resourcequotas`/`limitranges`) y `rbac.authorization.k8s.io` con `get`/`list`/`watch` y nada mas. ⚠️ **No** retirar contra un upstream que tenga los permisos namespaced en un `Role`: esa es exactamente la forma rota |
| 3 | **HIGH** · `POST /rca-agent/analyze` requiere authn + identidad de servicio | `agents/sre-agent/openapi.yaml`, `agents/sre-agent/src/api/agent_routes.py`, `agents/sre-agent/src/auth/__init__.py`, `agents/sre-agent/src/auth/dependencies.py`, `agents/sre-agent/tests/test_agent_routes.py`, `agents/sre-agent/tests/test_auth.py` | Endpoint de análisis aceptaba requests sin autenticación. Ahora exige JWT con `require_authn` y subject type `service_account` según `auth-config.yaml` | [openchoreo/openchoreo#4509](https://github.com/openchoreo/openchoreo/pull/4509) | Retirar cuando el PR upstream esté mergeado y el pin upstream contenga el fix |
| 4 | **HIGH** · bootstrap de authz revoca CRs removidos | `install/helm/openchoreo-control-plane/templates/authz/_authz-bootstrap.tpl`, `install/helm/openchoreo-control-plane/templates/authz/bootstrap-job.yaml`, `install/helm/openchoreo-control-plane/templates/authz/bootstrap-rbac.yaml`, `install/helm/openchoreo-control-plane/tests/verify-authz-bootstrap-prune.sh` | Sacar un binding/rol de `values.yaml` no revocaba acceso porque el bootstrap sólo hacía apply. Ahora los CRs generados tienen `openchoreo.dev/managed-by` y el Job usa `kubectl apply --prune` con selector y allowlists de los cuatro CRDs de authz | [openchoreo/openchoreo#4508](https://github.com/openchoreo/openchoreo/pull/4508) | Retirar cuando el PR upstream esté mergeado y el pin upstream contenga el fix |
| 5 | Backstage expone audience OIDC al portal | `install/helm/openchoreo-control-plane/templates/backstage/deployment.yaml`, `install/helm/openchoreo-control-plane/values.yaml`, `install/helm/openchoreo-control-plane/values.schema.json` | El portal ahora puede pedir `audience`, pero el chart no renderizaba `OPENCHOREO_AUTH_AUDIENCE`. Sin esa variable, Auth0 puede seguir emitiendo access tokens opacos para el login del portal. | [openchoreo/openchoreo#4518](https://github.com/openchoreo/openchoreo/pull/4518) | Retirar cuando el PR upstream esté mergeado y el pin upstream contenga el cambio |
| 6 | `extraRoles` aditivo en el bootstrap de authz | `install/helm/openchoreo-control-plane/templates/authz/_authz-bootstrap.tpl`, `install/helm/openchoreo-control-plane/values.yaml`, `install/helm/openchoreo-control-plane/values.schema.json` | El bootstrap sólo acepta roles por `bootstrap.roles`, y Helm **reemplaza** listas en vez de mergearlas: para agregar un rol propio (`component-deployer`, D14) habría que copiar los 11 roles del upstream —458 líneas— a nuestros values y dejar de heredar en silencio todo cambio del upstream en el próximo bump. `bootstrap.extraRoles` se **agrega** a `bootstrap.roles` en vez de pisarla, y hereda el label `openchoreo.dev/managed-by` del bootstrap, así que el `--prune` del parche 4 lo administra igual que a los demás | [openchoreo/openchoreo#4539](https://github.com/openchoreo/openchoreo/pull/4539) | Retirar cuando upstream acepte una forma aditiva de aportar roles al bootstrap (este `extraRoles` u otra equivalente) y el tag pinneado la incluya |

**Por qué 0b tiene fila.** Son archivos nuevos: no generan conflicto en el rebase y no consumen
presupuesto de diff. Pero la regla de arriba dice **todo** lo que difiere del upstream, y
`git diff upstream-main main` los lista. Sin fila, el próximo que corra ese diff no puede distinguir
"esto está registrado" de "esto se coló". Falta detectada por T08 sobre el trabajo de T00.

## Verificación

```bash
.github/scripts/verify-fork.sh              # el espejo esta como dice este archivo
.github/scripts/verify-fork.sh --self-test  # ¿el verificador detecta lo que dice detectar?
.github/scripts/verify-cluster-agent-rbac.sh # requiere helm + yq; RBAC del cluster-agent acotado en los 3 charts
```

Los dos comandos `verify-fork.sh` no tienen dependencias (bash + git). Contestan las preguntas que
este archivo afirma y que hasta ahora nadie hacía cumplir:

1. El sha del pin que dice este archivo, ¿es el mismo al que apunta `upstream-main`? **Éste es el
   check que hubiera cazado el sha del objeto tag.**
2. Ese sha, ¿es un **commit** o el objeto de un tag anotado? (regla 7).
3. `v1.2.2^{commit}`, ¿coincide con el sha registrado?
4. Todo archivo con diff contra `upstream-main`, ¿tiene fila en **Parches vigentes**?

Es el **piso**, y es el mismo script en los tres espejos. En este repo, `idp-sync` (T08) hace la
versión completa y mucho más profunda de la pregunta 4, más los diffs de CRDs, `values.schema.json`,
CHANGELOG y `helm template`. Este script no lo reemplaza: corre en 200 ms, sin Python ni red, y sirve
como pre-commit y como sanity check del propio `idp-sync`.

## Parches planificados

Reservados por el plan de agentes. **No implementados todavía** — no borres la fila, completala.

| # | Parche | Archivos previstos | Motivo | Track |
|---|---|---|---|---|
| 5 | *(reservado)* resolver de JWT para Auth0 | — | Presupuestado en D2 por si el control plane rechaza el token por algo imprevisto. **Puede no hacer falta**: el resolver es configurable | T06 / D2 |

## Parches retirados

| # | Parche | Retirado en | Motivo |
|---|---|---|---|
| _(ninguno todavía)_ | | | |
