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
