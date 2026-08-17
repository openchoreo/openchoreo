# `idp-sync` — agente de sincronizacion con el upstream

Contesta **una** pregunta cuando el upstream saca un tag nuevo: *que se nos rompe si lo
tomamos*. No rebasea, no mergea, no aplica nada. Emite un PR con un semaforo.

> Track **T08** del plan de agentes. Contexto:
> `docs/08-infrastructure/idp-openchoreo/README.md` del repo `architecture`.

---

## Los seis checks, en orden de riesgo

El orden no es cosmetico: es el orden en el que hay que leerlos, y el primero es el que
mas veces va a bloquear un bump.

| # | Check | Que mira | Cuando es 🔴 |
|---|---|---|---|
| 0 | **Consistencia del pin** | que `upstream.json`, `PATCHES.md` y la rama `upstream-main` digan lo mismo | el tag fijado no existe |
| 1 | **CRDs** | los 37 CRDs de `config/crd/bases/` **y** las copias que empaquetan los charts | version removida, cambio de storage version, propiedad removida, campo que pasa a obligatorio, enum achicado, cambio de tipo, se deja de preservar unknown fields |
| 2 | **`values.schema.json`** | los 4 charts | clave removida **que nosotros seteamos**, tipo cambiado, enum achicado, clave nueva obligatoria |
| 3 | **CHANGELOG** | breaking changes y deprecaciones **de todas las secciones**, no solo la del tag nuevo | breaking en el rango, o una deprecacion vieja que vence justo en este bump |
| 4 | **Colisiones de parches** | cruce entre nuestro diff, los commits `PATCH:` y la tabla de `PATCHES.md` | upstream toco un archivo que parcheamos |
| 5 | **`helm template`** | render con nuestros values, antes y despues | recurso que deja de renderizarse, o **regresion** de render |
| 6 | **Revalidacion de CRs** | todos los CRs de `idp-platform` contra los CRDs **y los webhooks** del tag nuevo, en un k3d efimero | un CR es rechazado |

### Por que los CRDs van primero

Tres razones que se acumulan:

1. **No hay conversion webhooks.** Si una version deja de estar servida o de ser la de
   almacenamiento, los objetos ya guardados quedan ilegibles. No hay migracion automatica.
2. **`helm upgrade` NUNCA actualiza `crds/`** — solo `helm install` lo hace. El upgrade
   deja los CRDs viejos en su lugar mientras el chart nuevo empieza a emitir campos que
   la API server desconoce.
3. **El pruning es silencioso.** Si upstream saca una propiedad y nuestro CR la sigue
   seteando, la API la descarta sin error. El apply devuelve OK, el campo no existe, y se
   descubre en runtime.

Por eso *cualquier* cambio de CRD arrastra como minimo un paso manual
(`kubectl apply --server-side -f config/crd/bases/`), y el reporte lo repite en el
checklist final.

### Por que el CHANGELOG se lee entero

Ya nos paso: las Git secret APIs se anunciaron deprecadas en la seccion de **v1.1.0**
(«deprecated and removed in v1.2») y se removieron en v1.2 **sin una linea en la seccion
de v1.2**. Leer solo el release nuevo no las veia.

El check hace tres pasadas: el rango `(base, target]` completo (los releases intermedios
cuentan), las minas anunciadas en secciones viejas que vencen ahora, y los bullets con
verbo de remocion que el upstream no clasifico bajo «Breaking Changes».

---

## Semaforo

| | Significa | Que hacer |
|---|---|---|
| 🟢 | sin hallazgos | releer igual el diff del upstream; el agente mira seis superficies, no todo el repo |
| 🟡 | revision humana obligatoria | hay una decision o un paso manual pendiente |
| 🔴 | **no mergear tal cual** | cada hallazgo trae la accion que lo desbloquea |

Reglas de agregacion, dos:

- **El peor gana.** Un solo rojo tine el reporte. No hay promedios.
- **Un check salteado nunca es verde.** Si el k3d no levanta, el check 6 sale amarillo
  diciendo que no pudo mirar. "No se" y "esta bien" no son lo mismo.

---

## Correrlo a mano

```bash
python3 -m venv .venv && .venv/bin/pip install -r idp-sync/requirements-dev.txt

git remote add upstream https://github.com/openchoreo/openchoreo.git
git fetch upstream --tags

# Hay tag nuevo?
PYTHONPATH=idp-sync .venv/bin/python -m idp_sync --sync-dir idp-sync latest-tag

# Checks 0-5 (minutos, sin cluster)
PYTHONPATH=idp-sync .venv/bin/python -m idp_sync --sync-dir idp-sync \
  analyze --target v1.3.0 --platform-root ../idp-platform -o static.json

# Check 6 (necesita k3d + helm + kubectl)
PYTHONPATH=idp-sync .venv/bin/python -m idp_sync --sync-dir idp-sync \
  validate --target v1.3.0 --platform-root ../idp-platform -o k3d.json

# Semaforo
PYTHONPATH=idp-sync .venv/bin/python -m idp_sync --sync-dir idp-sync \
  render -i static.json -i k3d.json -m reporte.md
```

Tests del propio agente:

```bash
cd idp-sync && ../.venv/bin/python -m pytest tests -q
```

Corren en el workflow **antes** que los checks: el agente decide si un bump entra a
produccion, asi que si el agente esta roto su verde no vale nada.

---

## El workflow

[`.github/workflows/idp-upstream-sync.yml`](../.github/workflows/idp-upstream-sync.yml).
Semanal, los lunes a las 06:15 UTC, y a mano por `workflow_dispatch` (inputs: `target_tag`,
`base_tag`, `skip_k3d`, `force`).

```
detect ─→ self-test ─┬─→ analyze (checks 0-5)  ─┐
                     └─→ k3d     (check 6)      ─┴─→ report ─→ PR
```

`analyze` y `k3d` van en jobs separados porque el k3d es la pieza mas fragil y no puede
llevarse puesto el reporte. `report` corre con `always()`.

**Salida:** rama `idp/upstream-sync/<tag>` con el reporte humano en
`idp-sync/reports/<tag>.md`, y un PR cuyo cuerpo es el semaforo. El JSON del reporte no se
commitea: queda como artifact del workflow (`idp-sync-report-<tag>-json`). Si es rojo, el workflow
ademas falla, para que se vea en la lista de Actions y no solo dentro del PR.

### Secreto opcional

`IDP_PLATFORM_TOKEN` — acceso de lectura a `fondomp-production/idp-platform`. Sin el, los
checks 2, 5 y 6 no pueden cruzar contra nuestros values ni contra nuestros CRs, y lo
**dicen** en amarillo con un mensaje explicito en vez de dar un verde vacio. El valor va a
Infisical, nunca al repo.

---

## Configuracion — `upstream.json`

Fuente de verdad **maquinable** del pin. `PATCHES.md` es la fuente **humana**, y el
check 0 verifica que las dos coincidan.

Claves que se tocan seguido:

| Clave | Para que |
|---|---|
| `pinned.tag` / `pinned.commit` | el tag fijado. Al rebasear se actualizan los dos, aca **y** en `PATCHES.md` |
| `expected_crd_count` | 37 hoy. Si cambia, el check 1 lo marca |
| `owned_paths` | archivos nuestros por construccion. **Sin esto el check 4 se reporta a si mismo** como drift no documentado |
| `platform.values` | mapa chart → archivo de values en `idp-platform` (los crea el track T10) |
| `platform.cr_dirs` | de donde salen los CRs a revalidar |

⚠️ `pinned.commit` es `git rev-parse <tag>^{commit}`, **no** `git rev-parse <tag>`: los
tags del upstream son anotados, y a secas devuelve el sha del objeto tag.

---

## Al rebasear a un tag nuevo

1. Leer el reporte entero, no solo el semaforo.
2. Ejecutar el checklist que trae al final (backups, CRDs a mano, verificar los parches).
3. Actualizar `pinned` en `idp-sync/upstream.json` **y** las filas de `PATCHES.md`.
4. Si el numero de CRDs cambio, actualizar `expected_crd_count`.

## Limites conocidos

- Mira las seis superficies del brief. **No** revisa el codigo Go del upstream: un cambio
  de comportamiento sin cambio de CRD, values ni changelog pasa invisible.
- El check 5 compara `base_tag` vs `target_tag`, no `main` vs `target_tag`: aisla el delta
  del upstream. Que archivos parcheados movio el upstream lo contesta el check 4.
- El check 6 necesita que el control plane levante en k3d. Si no lo logra, los CRs se
  validan solo contra el schema y el reporte lo aclara: las `preRenderValidations` CEL de
  ComponentTypes y Traits no se ejercieron.
