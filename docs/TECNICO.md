# Documentación técnica — Bot de Cuatro en línea 8×7

> Documento vivo. La arquitectura final, el algoritmo (búsqueda, heurística, manejo del tiempo) y el reporte de cobertura se completan a medida que se implementan los pasos de `PLAN_BACKEND.md` y `PLAN_FRONTEND.md`.
>
> Los planes **no se reescriben** después del primer commit de código: todo lo que cambie respecto a ellos se explica en la sección "Desviaciones del plan".

## Desviaciones del plan

### D1 — Niveles de dificultad en la API local (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §1 listaba "niveles de dificultad" como fuera del alcance. `PLAN_FRONTEND.md` §1 no los incluía entre los opcionales. El contrato del motor en `PLAN_BACKEND.md` §4 era `ChooseMove(b Board, me Player, legal []Column, budget time.Duration) Result`.

**Qué cambia.**

- `POST /local/games` acepta `difficulty`: `"easy"` | `"medium"` | `"hard"`, opcional, por defecto `"hard"`. `GameState` devuelve `difficulty`. Detalle en `docs/api-local.md` y `docs/fixtures/game-state.example.json`.
- La dificultad **no cambia el algoritmo**: limita la profundidad máxima del iterative deepening.

  | `difficulty` | `MaxDepth` | Qué se siente |
  |---|---|---|
  | `easy` | 2 | Gana y bloquea cuatros inmediatos, no ve trampas a largo plazo |
  | `medium` | 6 | Ve amenazas a tres jugadas; se le puede ganar con planificación |
  | `hard` | 0 (solo tiempo) | El mismo bot del torneo |

- El contrato del motor pasa a `ChooseMove(b Board, me Player, legal []Column, opts Options) Result`, con `Options{Budget time.Duration; MaxDepth int}`; `MaxDepth = 0` significa sin límite.
- `internal/api` mapea `difficulty` → `MaxDepth`. El loop del árbitro (`internal/arbiter`) llama siempre con `MaxDepth = 0`.
- En el frontend, el selector de dificultad entra como opcional, después del historial de jugadas.

**Por qué.** Las bases listan "niveles de dificultad" como opcional de la aplicación web. Con la búsqueda por iterative deepening el costo es un parámetro: el mismo código se detiene antes. Se mantiene la exigencia de las bases de que "el algoritmo del torneo y el de la app deben ser el mismo código".

**Cómo se garantiza que no afecta al torneo.**

- El loop del árbitro no recibe `difficulty`: construye `Options` con `MaxDepth = 0`. Un test del paso 4 lo verifica.
- El motor tiene un test "respeta MaxDepth" (paso 6): con `MaxDepth = n`, `Result.Depth ≤ n`.

**Impacto en los pasos.**

- Paso 5 (`internal/api`): validar `difficulty`, con `400 validation` si está fuera del enum; guardarla en la partida y devolverla en `GameState`.
- Paso 6 (`internal/engine`): `Options.MaxDepth` en el iterative deepening, más el test "respeta MaxDepth".
- Se implementa después de todo lo obligatorio.

**Quién lo decidió.** Matias Montini (PO), a partir de una revisión de los planes. Los planes no se editaron para respetar las bases ("un plan escrito o reescrito después de programar" se califica como deficiente). La desviación queda explicada acá.
