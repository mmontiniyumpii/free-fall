# Documentación técnica — Bot de Cuatro en línea 8×7

> Documento vivo. La arquitectura final, el algoritmo (búsqueda, heurística, manejo del tiempo) y el reporte de cobertura se completan a medida que se implementan los pasos de `PLAN_BACKEND.md` y `PLAN_FRONTEND.md`.
>
> Los planes **no se reescriben** después del primer commit de código: todo lo que cambie respecto a ellos se explica en la sección "Desviaciones del plan".

## Arquitectura final

Un solo proceso de Go (`bot/cmd/bot`) atiende el torneo y la aplicación web con el **mismo motor**, como piden las bases ("el algoritmo del torneo y el de la app deben ser el mismo código").

```
 ┌──────────────┐  GET /api/next (long poll) ┌───────────────────┐
 │  Árbitro     │ ◄───────────────────────── │ internal/arbiter  │  goroutine del loop
 │  (HTTPS)     │ ── POST /api/move ───────► │ cliente + loop    │  (solo si ARBITRO_URL)
 └──────▲───────┘                            └─────────┬─────────┘
        │ GET /api/games/{id}                          │ EngineChooser: MaxDepth = 0
        │ (vía proxy de desarrollo)                    ▼
 ┌──────┴───────┐   /local/games... (:3000)  ┌───────────────────┐    ┌───────────────────┐
 │  App web     │ ─────────────────────────► │ internal/api      │───►│ internal/engine   │
 │  (Angular,   │ ◄──────── GameState ────── │ partidas en       │    │ ChooseMove        │
 │   :4200)     │                            │ memoria + mutex   │    │ (reentrante)      │
 └──────────────┘                            └─────────┬─────────┘    └─────────┬─────────┘
                    MaxDepth según la dificultad (D1)   │                        │
                                                       ▼                        ▼
                                             ┌──────────────────────────────────────────┐
                                             │ internal/game — reglas puras (sin HTTP)  │
                                             └──────────────────────────────────────────┘
```

**Arranque de `cmd/bot`.**

1. Lee la configuración (`internal/config`).
2. Hace un warm-up del motor de 1 s sobre el tablero vacío.
3. Si está definida `ARBITRO_URL`, lanza el loop del árbitro en su propia goroutine.
4. Siempre levanta la API local en `PORT` (3000).
5. Ante `SIGINT` o `SIGTERM`, apaga el servidor en forma ordenada.

**Capas del bot.** Cada paquete tiene una sola responsabilidad y las dependencias van en una sola dirección:

| Paquete | Responsabilidad | No conoce |
|---|---|---|
| `internal/game` | Tablero, jugadas legales, victoria y empate | HTTP, motor |
| `internal/engine` | `ChooseMove`: búsqueda con presupuesto de tiempo y profundidad máxima opcional | HTTP, árbitro, app |
| `internal/arbiter` | Parseo de `/api/next`, cliente HTTP con timeouts y loop con `recover` | app |
| `internal/api` | API local de la app (`docs/api-local.md`): partidas en memoria protegidas con `sync.Mutex` | árbitro |
| `internal/config` | Variables de entorno y su validación | — |

Herramientas que viven en el mismo módulo:

- `cmd/mock`: árbitro simulado fiel al OpenAPI.
- `cmd/selfplay`: dos motores contra el mock (D5).
- `cmd/covercheck`: umbral de cobertura (D12).
- `scripts/run-bot.sh` y `scripts/run-bot.ps1`: supervisor con reinicio automático.

**Aplicación web (`web/`, Angular 21).** Componentes standalone con signals, sin librerías de UI.

| Pieza | Rol |
|---|---|
| `App` | Contenedor: guarda el `GameState` que devuelve la API, la fase (inicio, partida o reproducción), los pedidos en curso y el reintento sin conexión |
| `Board`, `Status`, `Toolbar`, `History` | Presentación: tablero e interacción, bloque de estado, acciones del panel e historial |
| `Rules`, `Confirm`, `Confetti` | Modal de reglas, confirmación de volver al inicio y animación de victoria |
| `Replay` | Reproducción de partidas del torneo a partir del registro público del árbitro (D10) |
| `GameApi` | Clase abstracta implementada por `GameApiService` (HTTP real) y por `FakeGameApiService` (escenarios de prueba, D8) |

**El frontend no implementa reglas.** Legalidad, ganador, línea ganadora y empate llegan en el `GameState` o en el registro del árbitro. La única excepción es la gravedad, y se usa solo para dibujar (D7).

**Contratos entre las partes.**

- `docs/api-local.md` y `docs/fixtures/game-state.example.json`, entre el bot y la app.
- `docs/bases/arbitro-openapi.json`, entre el bot o la app y el árbitro.

## Algoritmo

El motor está en `bot/internal/engine`. Su única función pública es `ChooseMove(board, me, legal, Options{Budget, MaxDepth})`, que devuelve la columna elegida junto con la profundidad alcanzada, los nodos visitados, el valor de la jugada y el tiempo usado. El torneo y la app llaman a esta misma función.

### Búsqueda

**Negamax con poda alfa-beta** (`search.go`).

- **Negamax.** Es minimax escrito desde el punto de vista de quien mueve: el valor de una jugada es el negativo del mejor valor que consigue el rival desde la posición resultante. Así hay una sola función para los dos jugadores.
- **Alfa-beta.** `alpha` es lo mínimo que el jugador ya tiene asegurado y `beta` lo máximo que el rival le va a permitir. Cuando una jugada alcanza `beta`, el resto de las hermanas no se mira, porque el rival nunca dejaría llegar a esa posición. El resultado es el mismo que el de minimax completo, visitando muchos menos nodos.
- **Valor de una victoria:** `100 000 − ply`, donde `ply` es la cantidad de jugadas desde la raíz. Entre dos victorias se prefiere la más rápida, y entre dos derrotas, la más lenta.
- **Tablero lleno sin ganador:** vale 0.

**Iterative deepening** (`choose.go`). Se busca a profundidad 1, después a 2, después a 3, y así mientras quede tiempo. Se juega la mejor columna de la **última profundidad completa**; si una profundidad se aborta por tiempo, se descarta entera. La búsqueda también termina antes de tiempo en dos casos:

- cuando encuentra una victoria o una derrota forzada, porque buscar más hondo no cambia el resultado;
- cuando alcanza la cantidad de casillas libres.

**Orden de jugadas** (`ordering.go`). Alfa-beta poda más cuanto antes se prueba la mejor jugada.

- **En la raíz,** primero se prueba la mejor columna de la profundidad anterior (D4) y después el resto, del centro hacia los bordes.
- **En el resto del árbol,** se va del centro a los bordes: `3, 4, 2, 5, 1, 6, 0, 7`. Las columnas centrales participan en más líneas posibles: la 3 y la 4 están en 4 ventanas horizontales por fila, y la 0 y la 7 en una sola.

**Poda de jugadas perdedoras** (`search.go` y `candidates` en `ordering.go`, D16). En cada nodo interior, antes de recorrer los hijos:

1. Si el que mueve gana en una jugada, el nodo vale `100 000 − (ply + 1)` y no se explora nada más.
2. Si el rival tiene dos o más casillas ganadoras jugables, no hay defensa: el nodo vale la derrota, `−(100 000 − (ply + 2))`. Si tiene una sola, la única jugada que se explora es taparla.
3. Las jugadas que le dejan al rival una casilla ganadora justo encima van al final del orden. No se eliminan, porque puede no haber otras.

Es la técnica del solver de Pascal Pons ("anticipate losing moves"), adaptada a nuestra representación con arrays. Con ella, el motor llega a 11–13 de profundidad en el medio juego, contra 10 sin ella.

**Representación interna** (`position.go`). Se usan arrays fijos: `cells[56]`, ordenado por columnas, y `heights[8]`. Las jugadas se hacen y deshacen en el lugar con `drop` y `undo`. La búsqueda no pide memoria: `TestSearchDoesNotAllocate` y el benchmark dan `0 allocs/op`. En un Apple M4 recorre entre 2 y 3 millones de posiciones por segundo, y en el medio juego llega a profundidad 10–11 con 1 500 ms.

### Heurística

Cuando la búsqueda llega a su profundidad máxima sin ganador, `evaluate` (`evaluate.go`) puntúa la posición para quien mueve, recorriendo las **107 ventanas** de 4 casillas donde puede formarse un cuatro en línea:

| Dirección | Cuenta | Ventanas |
|---|---|---:|
| Horizontal | 7 filas × 5 posiciones | 35 |
| Vertical | 8 columnas × 4 posiciones | 32 |
| Diagonal ↗ | | 20 |
| Diagonal ↘ | | 20 |
| **Total** | | **107** |

Las ventanas se calculan una sola vez al cargar el paquete y nunca se modifican. `TestWindows` verifica que sean 107, cuántas hay de cada dirección y que cada una forme una línea recta.

| Patrón en una ventana | Peso |
|---|---:|
| 3 fichas propias + 1 vacía | **+50** |
| 2 fichas propias + 2 vacías | **+10** |
| 3 fichas del rival + 1 vacía | **−60** |
| Cada ficha propia en las columnas 3 o 4 | **+3** (−3 si es del rival) |

Una ventana con fichas de los dos jugadores ya no puede dar un cuatro en línea, así que no suma.

**Por qué estos pesos.**

- **Un tres abierto vale 5 veces un dos abierto,** porque está a una ficha de ganar. Un dos abierto todavía necesita dos jugadas propias sin respuesta.
- **El tres del rival pesa más que el propio (−60 contra +50).** Ante dos posiciones parecidas, el bot prefiere la que desarma una amenaza antes que la que arma otra. Perder una partida cuesta lo mismo que ganarla, pero una amenaza del rival que no se ve a tiempo es la forma más común de perder.
- **El bonus del centro es chico (+3).** En un tablero de 8 columnas no hay una columna central única: la 3 y la 4 son las que más ventanas tocan. El bonus solo desempata posiciones sin amenazas, sobre todo en la apertura, y nunca supera a un tres.
- **Los pesos no se ajustaron con experimentos.** Son los del plan (`PLAN_BACKEND.md` §5) y se validaron por resultados: 10 de 10 victorias contra `casa-basico` (D4) y 30 partidas de self-play sin errores (D5).

### Manejo del tiempo

**Dos topes por jugada.** Con `THINK_MS = 1500`, el valor por defecto:

- **Presupuesto blando, en `THINK_MS`:** pasado ese tiempo no se empieza una profundidad nueva. La que está en curso puede terminar.
- **Parada dura, en `THINK_MS + 250 ms` (1 750 ms):** la profundidad en curso se aborta y se juega la de la anterior. El plan decía 1 800 ms; con ese valor las jugadas llegaban al árbitro con 1 800–1 802 ms. Por eso se bajó a +250 ms (D4).

En una partida real del mock (ver el ejemplo de abajo) se ven los dos casos:

- jugadas de **1 750 ms**: una profundidad nueva empezó antes de los 1 500 ms y se abortó en la parada dura;
- jugadas de **1 533 ms** y **1 609 ms**: la profundidad 11 terminó pasados los 1 500 ms y no se empezó la 12.

**El reloj se consulta cada 2 048 nodos.** `time.Now()` en cada nodo sería demasiado caro. A 2–3 millones de nodos por segundo, 2 048 nodos son menos de 1 ms, así que la parada dura se respeta con ese margen. `TestRespectsBudget` lo verifica.

**Warm-up.** Antes de entrar al loop del árbitro, `cmd/bot` hace una búsqueda de 1 s sobre el tablero vacío. Así, la primera jugada del torneo no paga el arranque en frío: código y memoria que todavía no están en caché. Por ejemplo, en el log de arranque de la partida del ejemplo dice `engine warm-up depth=10 nodes=2930688 elapsed_ms=1250`.

**Por qué no se usa `served_at`.** El árbitro cuenta los 3 000 ms desde `served_at`, con su propio reloj. Restar `served_at` a la hora local mezclaría dos relojes distintos, y cualquier desfase entre las máquinas se trasladaría completo al cálculo: con un reloj local adelantado, el bot creería que ya no tiene tiempo. En cambio, el bot mide desde que recibe el turno (`received := time.Now()` en `internal/arbiter/loop.go`) y deja 1 250 ms de margen para la red y el parseo. `served_at` se guarda solo para los logs. Con 300 ms de latencia simulada, el peor `ms` medido por el mock fue 2 069 (D5).

### Red de seguridad y guard final

Antes de buscar, `ChooseMove` hace tres comprobaciones baratas:

1. **Si hay una sola columna legal,** la juega sin buscar.
2. **Si puede ganar en esta jugada,** gana.
3. **Si el rival gana en su próxima jugada en alguna columna,** la bloquea.

La búsqueda también encontraría estas jugadas, pero así se resuelven en microsegundos y no dependen de que la búsqueda llegue a completar la profundidad 1 o 2. `TestTactics` y `TestSafetyNetBlocksWithoutSearch` las prueban.

**La jugada siempre pertenece a `legal_moves`.**

- La raíz solo prueba columnas que están a la vez en `legal_moves` y libres en el tablero.
- Al final, `guard` comprueba que la columna esté en `legal_moves` y, si no, devuelve la primera legal. Esto cubre el caso de que el árbitro mande un `legal_moves` que no coincida con el tablero.
- El loop del árbitro agrega dos capas más: su propio guard, y un `recover` que juega la primera columna legal si el motor entra en `panic`.

`TestNeverIllegal` (tableros al azar) y `TestGuardUsesGivenLegalMoves` lo prueban.

### Dificultad

La dificultad de la app **solo limita `MaxDepth`**: Fácil busca a profundidad 2, Intermedio a 6 y Difícil sin límite (`maxDepthFor` en `internal/api/games.go`). El presupuesto de tiempo, la heurística, la red de seguridad y el guard son los mismos en los tres niveles.

**El torneo siempre juega sin límite:** `arbiter.EngineChooser` llama con `MaxDepth = 0` y no lee ninguna dificultad (D1). `TestRespectsMaxDepth` y `TestDeterministicWithMaxDepth` cubren el límite.

### Fuentes y qué se adaptó a 8×7

Regla 3 de las bases: se citan las técnicas tomadas de otros.

- **Pascal Pons, "Solving Connect 4: how to build a perfect solver"** (blog.gamesolver.org).
  - **Se tomó** la estructura negamax + alfa-beta, el orden de columnas desde el centro, el iterative deepening y el valor de victoria que depende de la cantidad de jugadas.
  - **No se tomó** la representación con bitboards, la tabla de transposición ni la búsqueda con ventana nula: son la fase 2 del plan y no se implementaron (`TT_ENABLED` todavía no tiene efecto). Pons arma un solver exacto para 7×6. Este motor es heurístico y con tiempo acotado, porque para 8×7 no hay tablas públicas ni tiempo para resolver la posición completa.
- **Knuth y Moore, "An analysis of alpha-beta pruning",** *Artificial Intelligence* 6(4), 1975. Es la justificación de que la poda no cambia el resultado de minimax, y de que el ahorro depende del orden de las jugadas. Por eso se invirtió en el orden.
- **Chess Programming Wiki:** páginas Alpha-Beta, Iterative Deepening y Move Ordering. De ahí salen la técnica de probar primero la mejor jugada de la iteración anterior y la de descartar la profundidad abortada.
- **Zobrist (1970)** figura en el plan, pero solo para la tabla de transposición de la fase 2. No se usa.

**Adaptación propia al tablero de 8 columnas × 7 filas.**

- Las ventanas: 107, contra las 69 del 7×6 clásico.
- Dos columnas centrales (3 y 4) en lugar de una, tanto para el orden (`3, 4, 2, 5, 1, 6, 0, 7`) como para el bonus.
- Los pesos de la heurística.
- El tope de profundidad en 56 casillas.

### Ejemplo: por qué el bot jugó la columna 1

Partida de práctica contra `casa-basico` en el mock, jugada el 4 nov 2026 con el código de `bot/` del commit `1c2e508`:

- mock: `cmd/mock -seed 7`;
- práctica: `POST /api/practice {"opponent":"casa-basico","first":2}`;
- el bot es el jugador **2** y `casa-basico` abre;
- `casa-basico` gana si puede, bloquea si tiene que hacerlo, y si no juega al azar.

Así quedó el tablero antes del turno 12 del bot. Las filas 4 a 6 están vacías:

```
fila 3   .  .  .  2  .  .  .  .
fila 2   .  .  2  2  .  .  .  .
fila 1   .  .  2  1  .  .  1  .
fila 0   .  1  1  2  .  .  1  1
col      0  1  2  3  4  5  6  7
```

Jugadas hasta ahí (columna de cada turno): `6 3 2 2 3 3 7 2 1 3 6`.

**Lo que ve la heurística (profundidad 1).**

| Columna | 0 | **1** | 2 | 3 | 4 | 5 | 6 | 7 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Valor | 16 | **76** | 16 | 19 | −31 | −34 | −34 | −44 |

Las casillas se escriben como (fila, columna). La columna 1 pone una ficha en (1,1) y completa la diagonal ↗ (1,1)-(2,2)-(3,3) con las dos puntas vacías: (0,0) y (4,4). Son dos ventanas de "3 propias + 1 vacía", +50 cada una. Además, (0,0) se puede jugar enseguida: es una amenaza de ganar en el próximo turno.

**Lo que ve la búsqueda.** Con el iterative deepening, la columna 1 es la mejor en casi todas las profundidades. En la 2 gana la columna 2, pero en la 3 vuelve la 1.

En la **profundidad 9**, las columnas 0, 1, 2 y 3 valen **99 991**, es decir `100 000 − 9`: el bot gana en 9 jugadas, defienda como defienda el rival. Las columnas 4 a 7 valen entre 15 y 252: con ellas no hay una victoria forzada a 9 jugadas.

Entre cuatro empates, se queda con la columna 1, porque fue la primera en probarse: era la mejor de la profundidad 8, y en la raíz un empate no reemplaza a la mejor jugada ya encontrada.

Al encontrar una victoria forzada, el motor deja de profundizar. Por eso esta jugada tardó **206 ms**, contra los 1 500–1 750 ms de las anteriores. El log del turno dice `depth=9 nodes=473026 think_ms=206`.

**Lo que pasó después.**

| Turno | Quién | Columna | Comentario |
|---|---|---:|---|
| 13 | casa | 0 | Bloquea la punta (0,0) de la diagonal |
| 14 | bot | 2 | `depth=7`, 6 ms: la victoria forzada sigue, ahora a 7 jugadas |
| 15 | casa | 2 | |
| 16 | bot | 3 | `depth=5` |
| 17 | casa | 3 | |
| 18 | bot | 1 | `depth=3`: arma dos amenazas apiladas en la columna 0, la diagonal ↗ en (1,0) y la fila 2 en (2,0) |
| 19 | casa | 0 | Bloquea (1,0), y con esa ficha deja jugable (2,0) |
| 20 | bot | 0 | `depth=1`, 0 nodos: la red de seguridad gana sin buscar |

El bot gana en el turno 20 con la fila 2, columnas 0 a 3 (`winning_cells` `[[2,0],[2,1],[2,2],[2,3]]`), exactamente 9 jugadas después de la decisión del turno 12. La profundidad de cada jugada del bot baja de a dos (9, 7, 5, 3, 1), porque cada par de jugadas acerca la victoria en dos.

Los valores por columna salen de un test temporal que no se commiteó: llama a `negamax` sobre este tablero para cada columna, a profundidad 1, 5, 7 y 9. Este ejemplo se midió con el motor anterior a la poda de jugadas perdedoras (D16). Con el motor actual, la cantidad de nodos y la profundidad a la que aparece la victoria forzada pueden ser menores.

## Reporte de cobertura

Se obtiene con `npm test` desde la raíz, que corre `go test ./... -race -cover`, después `covercheck` y por último los tests de Angular. Los valores son del 4 nov 2026, sobre el commit `ef8aac5`.

**Bot (Go).** `covercheck` exige 70 % a `internal/game` y a `internal/engine` por separado (D12). Los dos superan el umbral con margen.

| Paquete | Cobertura | Exigido |
|---|---:|---|
| `internal/game` (reglas) | **100,0 %** (43/43 sentencias) | ≥ 70 % |
| `internal/engine` (algoritmo) | **97,0 %** (159/164 sentencias) | ≥ 70 % |
| `internal/api` | 95,7 % | — |
| `internal/arbiter` | 94,4 % | — |
| `internal/config` | 97,6 % | — |
| `cmd/covercheck` | 96,2 % | — |
| `cmd/mock` | 91,7 % | — |
| `cmd/selfplay` | 29,6 % | — |
| `cmd/bot` | 16,7 % | — |

`cmd/bot` y `cmd/selfplay` son programas de arranque. Su lógica vive en los paquetes de `internal/`, que sí están cubiertos.

**Casos que piden las bases y dónde se prueban.**

- **Reglas.** La caída de fichas está en `TestApply`, las jugadas legales en `TestLegalMoves` y el empate en `TestIsDraw`. `TestFindWin` cubre las cuatro direcciones: horizontal, vertical y las dos diagonales.
- **Algoritmo.** Nunca juega ilegal (`TestNeverIllegal`) y respeta el presupuesto de tiempo (`TestRespectsBudget`). Los tableros límite están en `TestOnlyOneLegalColumn` y `TestBoardWith55Cells`.
- **Cliente del árbitro.** El parseo usa los ejemplos del OpenAPI (`TestParseNext*`). El cliente se prueba contra un árbitro simulado con `httptest.NewServer`: construye la jugada y maneja los errores, los timeouts y el loop.

**Aplicación web (Angular, Vitest).** Tiene 161 tests en verde. No reporta cobertura, porque medirla requiere agregar `@vitest/coverage-v8` (D11). Cubren, entre otras cosas:

- la orientación del tablero (fila 0 abajo);
- que una columna llena no se pueda jugar;
- el reintento sin jugar dos veces;
- la línea ganadora;
- la reproducción, comparando contra el `board` del árbitro.

Varios de estos tests se comprobaron rompiendo el código a propósito y viendo que fallan (ver D7, D9 y D10).

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

### D2 — `FindWin` devuelve posiciones, no `Cell` (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §4: `FindWin(b Board, row int, c Column) []Cell`.

**Qué cambia.** `FindWin` devuelve `[]Pos`, con `type Pos struct{ Row, Col int }` agregado en `internal/game/types.go`. Devuelve exactamente 4 posiciones que incluyen la ficha jugada, ordenadas de abajo hacia arriba. `internal/api` las convierte a `winning_cells: [[fila, columna], ...]`.

**Por qué.** En el mismo plan, `Cell` es el *contenido* de una casilla (0, 1 o 2), no una posición. Un `[]Cell` no puede representar la línea ganadora.

### D3 — Firma de `RunLoop` y validación de la configuración del árbitro (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §4: `RunLoop(ctx, client, choose, logger)`.

**Qué cambia.**

- La firma pasa a `RunLoop(ctx, client, choose, logger, LoopOptions{Wait, Connected})`.
  - `Wait` es `POLL_WAIT`.
  - `Connected` es un `*atomic.Bool` que refleja si la última llamada al árbitro funcionó, y lo lee `GET /health` para `arbiter_connected`.
- `choose` es un `ChooseFunc func(Turn) Choice`, donde `Choice{Column, Depth, Nodes}` lleva los datos que pide el log por turno.
- `internal/config` rechaza arrancar si `ARBITRO_URL` está definida y no es una URL http(s), o si falta `TEAM_TOKEN`.

**Por qué.** Sin `Wait` no se puede configurar el long poll. Sin `Connected`, `/health` no puede informar el estado real de la conexión. Arrancar sin token deja al bot recibiendo `401` en silencio hasta perder por ausencia.

### D4 — Parada dura del motor a `THINK_MS + 250 ms` y orden con la mejor jugada previa (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §6 fija el presupuesto en `THINK_MS = 1500` y la parada dura en 1 800 ms. §7, paso 7, pide como criterio de listo que no haya jugadas de más de 1 800 ms. §5 define el orden de jugadas como "centro primero".

**Qué pasó.** Con la parada dura en 1 800 ms, casi todas las jugadas llegaban al mock con `ms` entre 1 800 y 1 802. Eso pasa porque el iterative deepening arranca una profundidad nueva poco antes de los 1 500 ms, esa profundidad se corta a los 1 800, y el `ms` del árbitro suma además parseo y red.

**Qué cambia.**

- La búsqueda se aborta en `Budget + 250 ms`, es decir 1 750 ms con el valor por defecto. Así la jugada completa llega antes de 1 800 ms.
- El presupuesto blando no cambia: no se empieza una profundidad nueva pasado `THINK_MS`.
- En la raíz se prueba primero la mejor jugada de la profundidad anterior, y después el resto desde el centro. Es la técnica estándar de iterative deepening y mejora la poda.

**Evidencia.**

- Antes del cambio: 10/10 victorias contra `casa-basico` en el mock, con `ms` máximo de 1 802.
- Después del cambio: 10/10 victorias, con `ms` máximo de 1 751 y la misma profundidad típica (10–11). Fue la misma semilla del mock, 5 partidas abriendo y 5 respondiendo.

### D5 — Criterio de tiempo del self-play y partidas entre equipos en el mock (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §7, paso 7: "dos motores contra el mock, latencia 300 ms, 30 min", con el criterio "sin errores ni jugadas > 1 800 ms; `-race` limpio".

**Qué cambia.**

- **El criterio de tiempo pasa a ser `ms` ≤ 1 800 + latencia simulada**, o sea 2 100 ms con 300 ms de latencia.
  - El `ms` del árbitro se mide desde `served_at`, así que incluye la latencia de red.
  - Con 300 ms simulados, una jugada de 1 750 ms de cómputo (ver D4) llega en unos 2 050 ms.
  - El criterio original y la latencia del mismo paso no se pueden cumplir a la vez.
  - El resumen del self-play informa además el `ms` máximo absoluto contra el límite real de 3 000 ms, que es el que hace perder la partida.
- **El mock agrega `POST /mock/friendly {"player1", "player2", "opening"}`.** Es una extensión del mock con prefijo `/mock`, y **no** forma parte del OpenAPI. Crea partidas entre dos equipos registrados, para que dos motores jueguen entre sí.
  - `opening` son hasta 4 jugadas al azar que no terminan la partida. Sin ellas, dos motores deterministas repetirían siempre las mismas dos partidas.
- **`cmd/selfplay` usa el mismo cliente, el mismo loop y el mismo motor que el bot.** El adaptador del motor (`arbiter.EngineChooser`, siempre con `MaxDepth = 0`) se movió de `cmd/bot` a `internal/arbiter` para que lo compartan los dos.

**Resultado de la corrida de 30 minutos.** Corrió contra el mock con `MOCK_LATENCY_MS=300` y `selfplay` compilado con `-race`, con la semilla 20261105.

- Se jugaron 30 partidas: 29 terminaron por `connect4` y 1 en empate. No hubo ningún timeout, jugada ilegal ni ausencia.
- El jugador que abre ganó 20 y el que responde 9.
- El `ms` máximo fue 2 069 y ninguna jugada superó el límite de 2 100.
- Hubo 0 logs `ERROR`, 0 `WARN` y 0 `DATA RACE`.
- Con apertura vacía, dos motores deterministas repiten la misma partida (las partidas 6 y 8 fueron idénticas). La apertura al azar evita que eso pase en el resto.

### D6 — Aplicación web: diseño de pantallas como referencia, ajustado a las bases (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §2 pide iconos Phosphor. El boceto de §3 pone "quién abre" y "nueva partida" en una barra superior. §1 deja el responsive como opcional y no incluye dificultad, color ni tema.

**Qué cambia.**

- La referencia visual es `docs/diseno/pantallas.html`, generado con Claude Design: escritorio y teléfono, ocho estados, notas de medidas y animaciones. La configuración de la partida va en un panel lateral, como en el diseño.
- Donde el diseño no alcanzaba para las bases, ganan las bases:
  - **Laptop.** El diseño fija la celda en 80 px (unos 800 px de alto). La celda se escala con el alto y el ancho de la ventana; en 1366×657 útiles mide 63 px y el tablero entra completo.
  - **Sin errores ni pedidos externos.** El diseño carga Lucide desde unpkg y las fuentes desde Google. Las fuentes (Geist, Geist Mono, Archivo, OFL) se sirven desde `web/public/fonts/` y la docena de iconos son SVG de Lucide (ISC) copiados en línea. No se usa Phosphor: no hace falta ninguna dependencia de iconos.
  - **Ningún control que no funcione.** Deshacer queda afuera hasta que exista `POST /local/games/{id}/undo`. El selector de color y el tema oscuro (opcionales) no se implementaron; los tokens del tema oscuro quedan en `styles.css`.
  - **Textos fieles al motor.** Las descripciones de dificultad del diseño ("se equivoca seguido") no corresponden a lo que hace el bot; se reescribieron según D1. Por defecto: Difícil, como el torneo.
- **Teléfono.** El toque en dos pasos (previsualizar y soltar) se activa por capacidad del dispositivo (`hover: none`), no por el ancho, para que una ventana angosta en la computadora siga soltando con un clic.

**Error de la IA detectado.** El prototipo del diseño dibuja la fila 0 arriba e implementa sus propias reglas (`findLine`, `botMove`). No se portó nada de eso; un test del tablero falla si se invierten las filas.

### D7 — Gravedad solo para dibujar y dueño de cada jugada leído del tablero (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §1 y §5: el frontend no implementa reglas y no infiere nada que no venga del backend (ganador, línea, columnas legales).

**Qué cambia.** Hay una excepción acotada: `landingRow(board, column)`, una función pura que devuelve la fila vacía más baja de una columna. Se usa solo para dibujar:

- la vista previa y el contorno de dónde cae la ficha, y el texto "cae en la fila N";
- tu ficha mientras el bot piensa (ver D9);
- los pasos de la reproducción de partidas (ver D10);
- la API falsa (ver D8).

Legalidad (`legal_moves`), ganador, línea ganadora y empate siguen saliendo siempre del backend o del árbitro.

El historial necesita saber quién hizo cada jugada, y `history` solo trae columnas. En vez de suponer turnos alternados, el dueño se lee del `board` del backend: la jugada en la columna `c` que es la `k`-ésima en esa columna corresponde a `board[k][c]`.

**Por qué.** Las bases califican "feedback inmediato". Sin esto, tu ficha aparecería recién cuando responde el bot, hasta ~1,8 s después.

**Cómo se verifica.** Tests de `landingRow` y de `moveHistory`. El test del historial falla si se reemplaza la lectura del tablero por turnos alternados.

**Quién lo decidió.** Matias Montini, con el límite explícito de "solo para dibujar".

### D8 — API falsa con escenarios elegibles por URL (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §5: mientras la API no existe, un servicio falso que devuelve un `GameState` fijo.

**Qué cambia.**

- La app usa la API real por defecto. El servicio falso queda en el código y se activa solo con `?scenario=`:
  - `playing`
  - `fixture`
  - `won`
  - `bot_won`
  - `draw`
  - `illegal_move`
  - `offline`, donde la primera jugada se aplica pero la respuesta se pierde
  - `lost`, que responde 404
- El bot falso tarda 1 500 ms, como el motor. Responde en un orden fijo de columnas (3, 4, 2, 5, 1, 6, 0, 7) y no detecta ganador.
- Mientras el fake está activo, la barra superior muestra la etiqueta **"Bot simulado (demo)"**. Se agregó después de que, en una prueba manual, se confundió al fake con el bot real: el fake apilaba fichas y no cortaba la partida con cuatro en línea.

**Por qué.** Hay estados que no se pueden provocar a mano: sin conexión, partida perdida, empate. Se necesitan para la demo, para las capturas del manual y para los tests.

### D9 — Tu ficha cae antes de la respuesta del bot (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §6, paso 6: "animación de caída simple".

**Qué cambia.**

- La caída sigue al diseño: 220 ms + 22 ms por fila y un rebote del 6 %.
- Tu ficha se dibuja cayendo apenas hacés clic, como jugada pendiente y sin tocar el `GameState`. La del bot cae cuando llega la respuesta.
- Las fichas nuevas se detectan comparando el tablero anterior con el nuevo, sin reglas.
- Un salto de más de 2 fichas (por ejemplo, al reproducir una partida) se muestra sin animar.

**Error detectado contra el backend real.** Con el fake (600 ms de demora) todo se veía bien. Contra el bot real, que respondía en milisegundos, la respuesta llegaba antes de que terminara la caída y la ficha aparecía de golpe. Lo detectó una medición con Playwright; se arregló manteniendo la animación en curso y se agregó un test que reproduce el caso.

**Reintento sin conexión.** Antes de reenviar una jugada, se consulta la partida y solo se reenvía si `history` no creció. Así una respuesta perdida nunca juega dos veces; hay tests para el caso.

### D10 — Reproducción de partidas del torneo a través de un proxy (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §1, opcional 4: reproducir una partida desde `GET /api/games/{id}`, que es público en el árbitro.

**Qué cambia.**

- El navegador no llama al árbitro directo: el mock no habilita CORS (el preflight responde 405) y no sabemos si el árbitro real lo hará.
- La app pide `/arbitro/api/games/{id}` a su propio servidor de desarrollo, y `web/proxy.conf.mjs` lo reenvía a `ARBITRO_URL`, la misma variable del bot. Sin esa variable, apunta al mock en `http://localhost:4000`.
- **Límite:** el proxy solo existe en el servidor de desarrollo (`npm run dev:web` o `npm --prefix web run start:lan`).
- El tipo del registro acepta `id` y `team_id` para el equipo. El schema `Team` del OpenAPI dice `id` y su ejemplo dice `team_id`.

**Cómo se verifica.**

- En los tests, el tablero del último paso tiene que ser igual al `board` del registro, con el ejemplo del OpenAPI y con una partida real de self-play guardada en `web/src/app/replay/testdata/`. Esos tests fallan si se invierten las filas.
- Contra el mock, el tablero final del navegador coincidió celda por celda con el `board` del árbitro.

### D11 — Pruebas del front en el comando único y presupuesto de estilos (4 nov 2026)

**Qué decía el plan.** `PLAN_FRONTEND.md` §7: el `npm test` de la raíz también corre los tests del front.

**Qué cambia.**

- `npm test` corre `test:bot`, que son los tests de Go con `-race` y `covercheck`, y después `test:web`, que son los tests de Angular (Vitest).
- Termina con código distinto de 0 si cualquiera de los dos falla.
- `npm install` en la raíz instala también las dependencias de `web/`, mediante `postinstall`.
- El front tiene 152 tests y no reporta cobertura: medirla necesita una dependencia nueva (`@vitest/coverage-v8`) que no se agregó. El umbral del 70 % de las bases aplica a reglas y algoritmo, que están en Go.
- El presupuesto de estilos por componente de `web/angular.json` pasa de 4 a 6 kB de advertencia y de 8 a 10 kB de error. `board.css`, con las animaciones del diseño, pesa 4,7 kB; partirlo solo para esquivar el límite hubiera ocultado su peso real.

### D12 — El umbral de cobertura se exige por paquete (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §4 y §7, paso 1: `cmd/covercheck` "falla si la cobertura de game+engine < 70 %".

**Qué cambia.** `covercheck` exige el 70 % a `internal/game` y a `internal/engine` **por separado**, no a la suma de los dos.

**Por qué.** Es más estricto: si cada paquete pasa el 70 %, la suma también. Con la suma, un `engine` al 95 % podría esconder un `game` al 30 %, y las bases piden cobertura de "reglas y algoritmo".

**Cómo se verifica.** Lo prueba `cmd/covercheck/main_test.go`, en el caso "one package below threshold fails even if combined is above". Hoy `npm test` informa `game` al 100 % (43/43 sentencias) y `engine` al 97 % (159/164).

### D13 — OpenAPI del árbitro 1.0.1, la versión final (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §7, paso 10, prevé el jueves a las 8:00 un diff del OpenAPI definitivo contra la versión preliminar 0.9.0 de las bases.

**Qué pasó.** La organización publicó la versión 1.0.1 la noche anterior, y el diff se hizo antes. El anuncio dice que no cambia ninguna ruta y que solo corrige la documentación. Al compararlas, el diff muestra más que eso:

- **Rutas públicas que ahora están documentadas:** `GET /api/games` (lista sin tablero, con `kind` y `limit`), `/api/state`, `/api/live` (SSE), `/api/health` y `/api/openapi.json`.
- **`Game` pasa a ser `GameSummary` + `board`.**
  - Los jugadores son `PlayerRef`, con `team_id` y no `id`. Así queda resuelta la duda de D10. Además traen `house: true` cuando el jugador es un bot de la casa.
  - Se agregan `series` y `current`.
  - `status` suma `scheduled` y `reason` suma `admin`.
  - `result` suma `forfeited_by`, `ms`, `column` y `waited_ms`.
- **`POST /api/move`:** una partida inexistente responde `409 unknown_game`, y antes respondía `not_your_turn`. Un cuerpo que no es JSON válido responde `400 validation` y no penaliza.
- **`POST /api/practice`:** `opponent` pasa a ser opcional, con `casa-basico` por defecto. Un rival inválido responde `400 validation`.
- **Otros cambios:**
  - `GET /api/me` trae `phase`, y `standing` puede ser `null` en los equipos de prueba.
  - Aparece la fase `frozen`.
  - `Submission` valida `repo_url` (solo github.com o bitbucket.org) y `commit` (hexadecimal, de 7 a 40 caracteres).
  - En amistosos, `match_id` vale `amistoso`.
- **`Turn` e `Idle`, lo único que usa el bot en el torneo, no cambian.**

**Qué cambia en el repo.**

- **`docs/bases/arbitro-openapi.json`:** se reemplaza por la 1.0.1.
- **`internal/arbiter/testdata/`:** se vuelven a extraer los ejemplos del OpenAPI.
  - Cambian `game.json` (con `series` y `current`) y `me.json` (con `phase`).
  - Se suman `error-unknown-game.json`, `error-invalid-json.json` y `health.json`.
  - El resto de los ejemplos son idénticos a los de la 0.9.0.
- **Bot:** no cambia el código. El loop ya trata cualquier `400` o `409` de `/api/move` como "no reintentar y seguir". `TestAPIErrors` suma los casos `unknown_game` y `validation`.
- **Mock (`cmd/mock`):** implementa todo lo anterior, con un test por cambio.
  - Incluye `GET /api/games` y `GET /api/health`.
  - **No implementa** `/api/state`, `/api/live` ni `/api/openapi.json`. Su schema es un objeto sin campos definidos, así que implementarlos obligaría a inventar la forma, y ni el bot ni la app los usan.
  - **Validación de `GET /api/games`:** un `kind` o un `limit` fuera de rango responden `400 validation`. El OpenAPI no documenta ese error, así que es una decisión del mock.
- **App (`web/src/app/replay`):**
  - El tipo `ArbiterGame` sigue el schema nuevo.
  - La reproducción explica las derrotas por `admin` y las partidas programadas (`scheduled`), con un test para cada caso.
  - El test que reproduce el ejemplo del OpenAPI ya lee la 1.0.1.

**Cómo se verifica.**

- `npm test` en verde.
- Un self-play contra el mock nuevo, sin errores.

### D14 — Reintento del `POST /api/move` y memoria de la última decisión (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §6 y `AGENTS.md` fijan un timeout de 5 s para el `POST /api/move`. Ante cualquier error, el loop duerme 1 s y vuelve a pedir turno.

**Qué pasó.** La revisión técnica (`docs/revision-bot.md`, punto 1.1) mostró que un solo corte de red en el `POST` hace perder la partida.

- El loop dormía 1 s y volvía a recibir el mismo turno.
- El motor pensaba otra vez 1 750 ms.
- La jugada llegaba a los **4,5 s** de la primera entrega, cuando el árbitro da 3 s.
- Además, el timeout de 5 s del `POST` era más largo que el plazo entero del árbitro.

**Qué cambia** (`internal/arbiter/loop.go`).

- **Reintento inmediato.** Si el `POST` falla por la red (no por un `400` o un `409` del árbitro), se reenvía **la misma columna**, con 50 ms entre intentos, hasta que pasen **2 600 ms desde la recepción del turno** (`moveDeadline`).
  - Es seguro: si un envío anterior sí llegó, el árbitro responde `409 not_your_turn`, que según el OpenAPI 1.0.1 no penaliza.
- **Cada intento vence en ese límite.** Usa un `context.WithTimeout` con el tiempo que queda, con un mínimo de 300 ms. Los 5 s del cliente quedan solo como tope de respaldo.
- **Memoria de la última decisión.** El loop guarda `game_id`, número de turno, columna y hora de recepción.
  - Si el árbitro vuelve a entregar ese mismo turno, juega la columna guardada sin volver a pensar.
  - El límite se sigue contando desde la primera recepción.
  - El estado vive dentro de `RunLoop`, no en una variable de paquete.
- **Log:** la línea de cada turno suma `attempts`. Cada reintento queda como `WARN move failed, retrying same column`.

**Evidencia.**

- `TestMoveIsRetriedAfterACutConnection`: el primer `POST` se corta con `Hijack` y el motor falso tarda 1 750 ms. La jugada se acepta a **1,81 s** de la primera entrega.
- `TestMoveRetriesStopAtTheDeadlineAndTheLoopSurvives`: si todos los intentos fallan, deja de reintentar al llegar al límite y el loop vuelve a pedir turno.
- `TestRedeliveredTurnReusesTheDecision`: un turno entregado otra vez no llama al motor.
- `TestReceptionToMoveUnder1800ms`: pasó de la suite de auditoría a la normal. Mide de la entrega al `POST` con el motor real: **1 755 ms**.
- `TestAuditLoopSurvivesHostileArbiter`, con `-tags audit`: de **4,504 s** pasa a **1,802 s**.

### D15 — El bot abre su puerto antes del warm-up y del loop del árbitro (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §4: `cmd/bot` hace warm-up, arranca el loop del árbitro y levanta la API local. El orden no estaba fijado.

**Qué pasó.** La revisión (punto 1.3) mostró que el bot abría el puerto **después** de lanzar el loop.

- Una segunda instancia en la misma máquina, por ejemplo alguien que corre `scripts/run-bot.sh` "por las dudas", llegaba a hacer long poll con nuestro token antes de morir por el puerto ocupado.
- El supervisor la relanzaba cada ~2 s.
- Si el árbitro le entregaba un turno a ese proceso efímero, el turno quedaba servido a un proceso que moría.

**Qué cambia** (`cmd/bot/main.go`).

- `main` delega en `run(ctx, cfg, warm)`.
- `run` hace `net.Listen(":"+PORT)` **antes** del warm-up y del loop, y después sirve con `srv.Serve(ln)`.
- Si el puerto está ocupado, termina con `ERROR bot stopped ... is another bot already running?` y código 1, sin haber hablado con el árbitro.
- El resto del arranque no cambia.

**Evidencia.**

- `TestBusyPortFailsBeforeTalkingToTheArbiter`: con el puerto ocupado, un árbitro simulado recibe **0** requests. Antes del cambio recibía **2 276** en 300 ms, con `POLL_WAIT = 0`.
- `TestRunServesHealthAndStopsOnCancel`: con el puerto libre, `run` sirve `/health` y termina sin error al cancelar.
- A mano, con dos binarios en el mismo `PORT`: el segundo sale enseguida con código 1.

**Lo que no cubre.** Dos máquinas con el mismo token. Para eso no hay fix de código: es una regla operativa (la segunda laptop, apagada; plan §8).

### D16 — Poda de jugadas perdedoras dentro del árbol (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §5: negamax con alfa-beta, con orden "centro primero" en todo el árbol. La mejora de profundidad estaba prevista para la fase 2 (bitboard y TT).

**Qué pasó.** La revisión (punto 1.2) mostró que el motor perdía 5 de 20 partidas contra sí mismo limitado a profundidad 4. Todas eran finales largos, con derrotas que estaban más allá de su horizonte de 10 plies.

**Qué cambia** (`internal/engine/search.go` y `ordering.go`). Se aplica en cada nodo interior; el detalle está en "Algoritmo", en el apartado de la búsqueda.

- Si se puede ganar en una jugada, el nodo vale la victoria.
- Si el rival tiene una amenaza jugable, la única jugada explorada es taparla; si tiene dos, el nodo vale la derrota.
- Las jugadas que le dejan al rival una casilla ganadora justo encima van al final del orden.

**Qué no cambia.**

- La heurística y sus pesos.
- La raíz: sigue con la red de seguridad y el orden con la mejor jugada previa.
- Sin asignaciones: `TestSearchDoesNotAllocate` y el benchmark siguen en 0 allocs/op.
- Sin goroutines y sin `time.Now()` por nodo.

**Cómo se midió.** Con `-tags audit`, 4 partidas en paralelo y un Apple M4. `baselineChoose`, en `internal/engine/audit_test.go`, es una copia literal de la búsqueda anterior.

| Criterio | Antes | Después |
|---|---|---|
| a. Suite táctica (`TestAuditTactics`) | 8/8 | **8/8** |
| b. Profundidad en las mismas posiciones, a 8 / 12 / 16 / 24 fichas (`TestAuditDepth`) | 10 / 10 / 10 / 11 | **13 / 11 / 12 / 12** |
| c. Motor nuevo contra baseline: 20 partidas, 10 aperturas de 2 jugadas con los dos colores (`TestAuditVsBaseline`) | — | **12 G / 0 E / 8 P** |
| d. Contra `casa-aleatorio` y `casa-basico` (`TestAuditSelfPlayHouse`) | 40/40 | **40/40** |
| e. Contra el mismo motor a profundidad 4 (`TestAuditSelfPlayDepth4`) | 15 G / 0 E / 5 P | **16 G / 0 E / 4 P** |

**Detalles de la medición.**

- **Velocidad.** Cada nodo hace más trabajo, así que los nodos por segundo bajan de ~2,4 M a ~2,0 M. Igual, la poda compensa con creces: con 32 fichas, la victoria forzada a 19 plies aparece en **89 ms**, contra 899 ms antes.
- **El color pesa en el criterio c.** De las 8 derrotas del motor nuevo, 7 fueron como jugador 2.
  - Como jugador 1, el nuevo ganó 9 de 10; baseline, 7 de 10.
  - Como jugador 2, el nuevo ganó 3 de 10; baseline, 1 de 10.
  - Es decir, el nuevo es mejor con los dos colores.
- **La muestra es chica.** 20 partidas no alcanzan para afirmar una mejora con significancia estadística, pero el criterio fijado (no perder más que baseline) se cumple con margen.

**Tests nuevos** (`prune_test.go`), todos con tableros dibujados:

- `TestNodeWithImmediateWinStopsAtOnce`: (1), 1 nodo en lugar de 97.
- `TestNodeFacingTwoThreatsIsLost`: (2) con dos amenazas, 1 nodo en lugar de 351.
- `TestNodeFacingOneThreatOnlyBlocks`: (2) con una amenaza, 2 nodos en lugar de 9.
- `TestMovesUnderARivalWinGoLast`: (3).

### D17 — `TT_ENABLED` no tiene efecto: la fase 2 no se hizo (4 nov 2026)

**Qué decía el plan.** `PLAN_BACKEND.md` §5 y §7, paso 9: bitboard y tabla de transposición, apagables con `TT_ENABLED` y validados con self-play contra la versión base.

**Qué pasó.** No se implementaron. `internal/config` lee y valida `TT_ENABLED`, pero ningún código lo usa. La mejora de profundidad para la noche anterior al torneo se hizo con la poda de jugadas perdedoras (D16), que no necesita estructuras nuevas y se prueba con tableros dibujados.

**Qué cambia.** Nada en el código. El README ya dice que la variable está reservada y no tiene efecto.

### D18 — El timeout de 5 s del `POST /api/move` es más largo que el plazo del árbitro (4 nov 2026)

**Qué decía el plan.** `AGENTS.md` y `PLAN_BACKEND.md` §6: `http.Client` con 5 s para `POST /api/move`.

**Qué pasó.** El árbitro da 3 000 ms desde `served_at`. Un `POST` colgado 5 s pierde la partida igual, y ese timeout no la protegía.

**Qué cambia.** El cliente conserva los 5 s como tope de respaldo, para no tocar `AGENTS.md` la noche anterior. Pero desde D14 cada intento de `POST` tiene un contexto que vence a los 2 600 ms de la recepción del turno, y ese límite es el que manda.

### D20 — El PDF de las bases no está en `docs/bases/` (4 nov 2026)

**Qué decía el plan.** `AGENTS.md` y `PLAN_BACKEND.md` dicen que las bases están en `docs/bases/`, en PDF y en `arbitro-openapi.json`.

**Qué pasó.** En el repo solo está `arbitro-openapi.json`. Por eso la revisión técnica no pudo verificar si las bases prohíben explícitamente correr más de un proceso por token (punto 1.3).

**Qué cambia.** Hay que agregar el PDF a `docs/bases/` cuando alguien del equipo lo tenga a mano. Mientras tanto, la regla operativa es una sola instancia conectada al árbitro (D15).
