# idp-openchoreo — espejo del control plane de OpenChoreo

> **Este no es el README del upstream.** El README original de OpenChoreo sigue intacto en
> [`../README.md`](../README.md); GitHub muestra éste porque `.github/README.md` tiene precedencia.
> Se hizo así a propósito: **no se toca ningún archivo del upstream para documentar el fork.**

Espejo de [`openchoreo/openchoreo`](https://github.com/openchoreo/openchoreo) (Apache-2.0), la base
del IDP de la organización. La marca del portal es **Kira**; este repo es el **control plane**:
controllers, CRDs, charts y la CLI `occ`.

## Antes de tocar nada

La fuente de verdad **no vive en este repo**. Vive en el repo `architecture`:

| Documento | Para qué |
|---|---|
| `docs/08-infrastructure/idp-openchoreo/README.md` | El brief maestro. **Leelo entero antes de escribir una línea.** |
| `docs/08-infrastructure/idp-openchoreo/decisiones.md` | Las decisiones cerradas, con su justificación |
| `docs/08-infrastructure/idp-openchoreo/plan-agentes.md` | Tracks, dependencias y estado |

## Principio rector

> **OpenChoreo manda. Lo nuestro va encima como extensión, nunca como reemplazo.**

En orden, ante cualquier duda de implementación:

1. Si se resuelve con **un CR o un value de Helm**, no se toca código → va a `idp-platform`.
2. Si hay que tocar código, se **agrega un archivo nuevo**; no se edita uno del upstream.
3. Si hay que editar un archivo del upstream, se manda **PR upstream en paralelo** y el parche lleva
   su **condición de retiro** escrita en [`PATCHES.md`](../PATCHES.md).

**Diff objetivo de este espejo: 2 parches de seguridad** (más la higiene de fork). Todo lo demás vive
en repos propios sin upstream.

## Ramas

| Rama | Qué es | Regla |
|---|---|---|
| `upstream-main` | Espejo puro del upstream en el tag fijado | **Nunca se toca.** Sólo avanza al rebasear a un tag nuevo |
| `main` | `upstream-main` + parches | Un parche = un commit con prefijo `PATCH:` en el subject |

## Pin actual

| | |
|---|---|
| Upstream | `openchoreo/openchoreo` |
| Tag | **`v1.2.2`** (release del 2026-08-06) |
| Commit | `58758d662fa07bf885acc22045c6919f4aa2e8c1` |

**Se sigue tags de release, nunca `main`.** El upstream mueve ~100 commits cada 2,5 meses con
refactors estructurales, no tiene umbral de coverage y su e2e no corre en CI.

## Cómo sincronizar con el upstream

```bash
git remote add upstream https://github.com/openchoreo/openchoreo.git   # ya configurado si clonaste
git fetch upstream --tags
git config rerere.enabled true      # recuerda resoluciones de conflicto entre bumps
```

**Antes de rebasear**, corré el agente de sincronización (track T08). Reporta, en orden de riesgo:

1. **Diff de los 37 CRDs** (`config/crd/bases/*.yaml`) — la superficie más peligrosa: no hay
   conversion webhooks y **Helm nunca actualiza `crds/` en `upgrade`**.
2. Diff de `values.schema.json` — las claves removidas rompen values en silencio.
3. Breaking changes del CHANGELOG, **incluyendo deprecations anunciadas en releases anteriores** que
   se materializan en ésta.
4. Colisión con parches: para cada archivo tocado por un `PATCH:`, ¿cambió upstream?
5. `helm template` con nuestros values, antes y después, y diff del render.
6. Revalidación de **todos** los CRs de `idp-platform` contra los webhooks nuevos, en un k3d efímero.

Rollback: backup de los 37 kinds + los Secrets del namespace del control plane antes de cada upgrade.
Los CRs son el estado autoritativo. **Los CRDs no se revierten solos.**

## Reglas de higiene

- **Nunca** reformatear, renombrar ni reorganizar archivos del upstream. Un `gofmt` masivo convierte
  cada merge futuro en un infierno.
- Todo lo que sea configuración va a `idp-platform`, no acá.
- Todo lo que sea cadena de suministro va a `idp-ci`, no acá.
- Nunca commitear secretos: en el repo van placeholders, los valores van a Infisical.

## GitHub Actions

**Deshabilitadas a nivel repo.** Los workflows del upstream (`release.yml`, `scorecard.yml`,
`e2e-gate.yml`, …) no deben correr bajo nuestra org: publicarían imágenes y firmarían releases con
nuestra identidad. El track **T08** las reactiva con una allow-list que sólo incluya nuestros
workflows.

## Repos hermanos

| Repo | Qué es |
|---|---|
| `idp-openchoreo-modules` | Espejo de `openchoreo/community-modules` (GCP, KEDA, OpenCost) |
| `idp-kira-portal` | Espejo de `openchoreo/backstage-plugins` (el portal, marca Kira) |
| `idp-platform` | Propio: ComponentTypes, Traits, ResourceTypes, Environments, values, políticas |
| `idp-ci` | Propio: `ClusterWorkflow` extendido + glue templates |

## Licencia

Apache-2.0, heredada del upstream. Ver [`../LICENSE`](../LICENSE).
