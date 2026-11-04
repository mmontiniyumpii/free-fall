# PLAN_BACKEND.md — Bot de Cuatro en línea 8×7

> Equipo 3. Challenge de IA aplicada al desarrollo, Yompi / Yalutec / Migo, 5 de noviembre de 2026.
> Este plan se escribe y commitea **antes de la primera línea de código**. Lo que cambie respecto a él se documenta en `docs/TECNICO.md`, sección "Desviaciones del plan".

Integrantes: Mauricio Batres, José Danilo Alvarado, Luis Fernando Rivas, Matias Montini.

## 1. Alcance

**Dentro del alcance**
- Reglas completas del juego 8×7 (caída de fichas, jugadas legales, victoria en 4 direcciones, empate a 56 casillas).
- Motor de búsqueda que elige una columna en ≤ 1 500 ms de cómputo (presupuesto configurable).
- Cliente del árbitro: long polling a `GET /api/next`, envío con `POST /api/move`, manejo de `idle`, errores y desconexión.
- API HTTP local para la aplicación web, que usa **el mismo motor y las mismas reglas** que el torneo.
- Mock del árbitro para desarrollar y probar antes del jueves.
- Pruebas automatizadas con un comando único y reporte de cobertura (≥ 70 % en reglas y motor).

**Fuera del alcance** (opcional, solo si sobra tiempo y en este orden)
- Bitboard `uint64` (fase 2), tabla de transposición (fase 2), libro de aperturas, niveles de dificultad, reproducción de partidas del árbitro.

## 2. Decisión de lenguaje: Go

Nadie del equipo ha trabajado con Go. Lo elegimos igual porque:
- El reto pide usar tecnologías que no conocemos y poder explicarlas; Go es chico y se lee como pseudocódigo.
- El bot corre el loop del árbitro y la API de la app en el mismo proceso: en Go cada request es una goroutine, así que una partida en la app no puede bloquear un turno de liga (en Node, de un solo hilo, sí).
- Velocidad: más profundidad de búsqueda en el mismo presupuesto, sin tocar el algoritmo.
- Librería estándar suficiente (`net/http`, `encoding/json`, `testing`, `-cover`): sin dependencias, un binario, igual en macOS y Windows.

**Spike de 30 minutos** antes de cualquier otra cosa: instalar Go en las cuatro máquinas, `go mod init`, un `main` que sirve `/health` y hace `GET` a un servidor local, un test con `go test -cover`. Si no funciona en los 30 minutos, se vuelve a TypeScript + Node y se anota en `docs/TECNICO.md`.

## 3. Arquitectura

```
 ┌───────────────┐   GET /api/next (long poll)   ┌──────────────────┐
 │   Árbitro     │ ◄──────────────────────────── │ internal/arbiter │  goroutine 1
 │ (público)     │ ──── POST /api/move ────────► │ (client, loop)   │
 └───────────────┘                               └────────┬─────────┘
                                                          │ engine.ChooseMove(board, me, legal, budget)
 ┌───────────────┐   POST /local/games/:id/moves  ┌───────▼─────────┐     ┌─────────────────┐
 │  App web      │ ──────────────────────────────►│ internal/api    │────►│ internal/engine │
 │  (Angular)    │ ◄──────────────────────────────│ (server, games) │     │ (negamax)       │
 └───────────────┘          GameState             └───────┬─────────┘     └────────┬────────┘
                                   goroutines del server  │                        │
                                                     ┌────▼────────────────────────▼───┐
                                                     │  internal/game (reglas puras)    │
                                                     └──────────────────────────────────┘
```

Principios: las reglas no conocen HTTP; el motor no conoce al árbitro ni a la app; el cliente del árbitro solo conoce `engine.ChooseMove`. El motor es **reentrante**: cada llamada trabaja sobre su propia copia de la posición, sin estado global mutable, para que las goroutines del árbitro y de la app no se pisen.

## 4. Estructura y contratos

```
bot/                         módulo Go  (go.mod: module github.com/mmontiniyumpii/free-fall/bot)
├── cmd/bot/main.go          arranca loop del árbitro + API local + warm-up
├── cmd/mock/main.go         árbitro simulado (mismo módulo: comparte las reglas)
├── cmd/selfplay/main.go     dos motores contra el mock, con latencia artificial
├── cmd/covercheck/main.go   falla si la cobertura de game+engine < 70 %
├── internal/game/           types.go, board.go, rules.go  (+ *_test.go)
├── internal/engine/         evaluate.go, search.go, ordering.go, choose.go  (tt.go, bitboard.go en fase 2)
├── internal/arbiter/        client.go, parse.go, loop.go, testdata/*.json (ejemplos del OpenAPI)
├── internal/api/            server.go, games.go
└── internal/config/         config.go (variables de entorno)
```

| Módulo | Responsabilidad | Contrato público |
|---|---|---|
| `internal/game` | Tablero, reglas, victoria, empate | `LegalMoves(b Board) []Column`, `Apply(b Board, c Column, p Player) (Board, int, error)`, `FindWin(b Board, row int, c Column) []Cell`, `IsDraw(b Board) bool` |
| `internal/engine` | Elegir jugada en tiempo acotado | `ChooseMove(b Board, me Player, legal []Column, budget time.Duration) Result{Column, Depth, Nodes, Score, Elapsed}` |
| `internal/arbiter` | Hablar con el árbitro | `ParseNext([]byte) (Turn, bool, error)`, `Client.Next(ctx, wait)`, `Client.Move(ctx, gameID, col)`, `RunLoop(ctx, client, choose, logger)` |
| `internal/api` | Partidas humano vs bot en memoria (mapa protegido por mutex) | `POST /local/games`, `POST /local/games/{id}/moves`, `GET /local/games/{id}`, `GET /health` |
| `internal/config` | Entorno | `ARBITRO_URL`, `TEAM_TOKEN`, `THINK_MS` (1500), `PORT` (3000), `POLL_WAIT` (20), `TT_ENABLED` (false) |
| `cmd/mock` | Árbitro simulado fiel al OpenAPI | Mismos endpoints; rival `casa-aleatorio`/`casa-basico`; `MOCK_LATENCY_MS` |

**Tipos compartidos** (`internal/game/types.go`), congelados desde el primer commit de código:

```go
type Cell uint8      // 0 vacío, 1 jugador 1, 2 jugador 2
type Player uint8    // 1 abrió, 2 respondió
type Column uint8    // 0..7
const Rows, Cols, Cells = 7, 8, 56
type Board [Rows][Cols]Cell   // Board[fila][columna], fila 0 = abajo (igual que el árbitro)
```

**`GameState` de la API local** (contrato con el frontend, ver `PLAN_FRONTEND.md`), JSON en snake_case:

```json
{ "id": "l_01", "board": [[0,0,0,1,2,0,0,0], "...6 filas más, vacías"], "to_move": 1, "legal_moves": [0,1,2,3,4,5,6,7],
  "status": "playing", "winner": null, "winning_cells": null, "history": [3,4],
  "human": 1, "bot": 2, "last_bot": { "column": 4, "depth": 9, "nodes": 812345, "elapsed_ms": 1310 } }
```

`status`: `"playing" | "won" | "draw"`; `to_move`: `1 | 2 | null` (`null` si terminó); `winner`: `1 | 2 | null`; `winning_cells`: `[[fila, columna] ×4] | null`, mismo formato que el árbitro. Cuerpos: `POST /local/games {"first": "human" | "bot"}`, `POST /local/games/{id}/moves {"column": 0-7}`. Errores: `{"ok": false, "error": "illegal_move" | "validation" | "unknown_game" | "game_over" | "not_your_turn", "message": "..."}` con 400 / 400 / 404 / 409 / 409, mismo formato que el árbitro. Detalle en `docs/api-local.md`.

## 5. Algoritmo elegido y por qué

**Negamax con poda alfa-beta + iterative deepening con control de tiempo + ordenamiento de jugadas + heurística por ventanas.** Fase 2 opcional: bitboard `uint64` y tabla de transposición.

- Determinista y explicable: un árbol de jugadas, se corta lo que no puede mejorar el resultado, se profundiza mientras hay tiempo.
- Connect 4 tiene ramificación ≤ 8 y partidas cortas: alfa-beta con buen orden (centro primero) poda muchísimo.
- Representación en arrays fijos (`[Cells]uint8` + alturas `[Cols]uint8`), `drop`/`undo` in place, sin asignaciones dentro de la búsqueda.
- Evaluación: 107 ventanas de 4 celdas precalculadas (35 horizontales, 32 verticales, 40 diagonales); 3 propias + 1 vacía = +50, 2 + 2 vacías = +10, 3 rivales + 1 vacía = −60, bonus por fichas en columnas 3 y 4. Victoria = 100 000 − ply.
- Red de seguridad previa a la búsqueda: ganar ahora si se puede; bloquear victoria inmediata del rival.
- Guard final: la columna devuelta siempre pertenece a `legal_moves`; si no, la primera legal.
- Fase 2 bitboard: cada columna son 8 bits (7 filas + 1 centinela) → el tablero de un jugador entra exacto en un `uint64`; 4 en línea se detecta con desplazamientos de 1, 7, 8 y 9. Solo si la persona del motor puede dibujarlo en una pizarra.
- Fase 2 TT: clave Zobrist `uint64`, guarda profundidad, tipo de cota, score y mejor jugada; se vacía por turno; apagable con `TT_ENABLED`.

Fuentes: Pascal Pons, "Solving Connect 4: how to build a perfect solver" (blog.gamesolver.org); Knuth y Moore, "An analysis of alpha-beta pruning", Artificial Intelligence 6(4), 1975; Zobrist, "A new hashing method with application for game playing", 1970; Chess Programming Wiki (Alpha-Beta, Iterative Deepening, Move Ordering). Adaptación propia a 8×7: ventanas, orden de columnas, pesos.

Descartados: MCTS (más difícil de calibrar en 1.5 s y de justificar jugada a jugada); solver completo (no hay tablas públicas para 8×7).

## 6. Manejo del tiempo y concurrencia

- El árbitro cuenta 3 000 ms desde `served_at` (su reloj). El bot **no** usa `served_at`: mide desde que recibe la respuesta.
- Presupuesto `THINK_MS = 1500`, parada dura a 1 800 ms. El resto es para red y parseo. Se calibra el jueves con `ms` de `GET /api/games/{id}`.
- Iterative deepening devuelve la jugada de la última profundidad completada. Reloj consultado cada 2 048 nodos (`time.Now()` es caro por nodo).
- Warm-up al arrancar: 1 s de búsqueda sobre tablero vacío.
- `http.Client` con `Timeout: 35 * time.Second` (el long poll dura hasta 20 s; sin timeout un socket colgado mata el bot en silencio).
- El loop del árbitro corre en su goroutine; el server HTTP en las suyas. El motor no tiene estado global; `api` protege su mapa de partidas con `sync.Mutex`. Tests con `-race`.
- Durante la liga nadie juega en la instancia conectada al árbitro: la demo se hace con una segunda instancia sin `ARBITRO_URL`.

## 7. Pasos en orden, con criterio de listo

| # | Paso | Responsable | Listo cuando |
|---|---|---|---|
| 0 | Spike Go: instalación, `go mod init`, `/health`, un `GET` local, `go test -cover` | Todos | Funciona en las 4 máquinas en ≤ 30 min; si no, vuelta a TS |
| 1 | Scaffold: `cmd/`, `internal/`, `cmd/covercheck`, comando único en la raíz, `.env.example`, `.gitignore` | Plomería | `npm test` (raíz) corre `go test ./... -race -cover` + covercheck sin errores |
| 2 | `internal/game` + tests table-driven | Motor | Tests de las 4 direcciones, columna llena, empate y tablero dibujado a mano en verde |
| 3 | `cmd/mock`: endpoints del OpenAPI, long poll real con canales, rival aleatorio y básico, latencia | Mock/QA | `curl` completa una partida contra el mock |
| 4 | `internal/arbiter`: `ParseNext` con `testdata/`, cliente con timeout, loop con jugada aleatoria; tests con `httptest.NewServer` | Plomería | Bot aleatorio termina una partida contra el mock sin errores |
| 5 | `internal/api`: partidas en memoria, 4 endpoints, CORS | Plomería | `curl` juega una partida humano vs bot |
| 6 | `internal/engine`: negamax + αβ + ID + orden + evaluación + guard | Motor (+ par) | Gana 10/10 a `casa-basico` del mock; tests "nunca ilegal" (1 000 posiciones aleatorias), "≤ presupuesto", "una sola columna legal" y "tablero con 55 casillas" en verde |
| 7 | `cmd/selfplay`: dos motores contra el mock, latencia 300 ms, 30 min | Mock/QA | Sin errores ni jugadas > 1 800 ms; `-race` limpio |
| 8 | Endurecer: supervisor (`scripts/run-bot.sh` y `.ps1`), logs por turno con `log/slog`, warm-up | Plomería | Matar el proceso → vuelve solo en < 5 s |
| 9 | Fase 2: bitboard y/o TT, apagables, validados con self-play contra la versión base | Motor | No pierde más que la base y busca más profundo |
| 10 | Jueves 8:00: diff OpenAPI definitivo, `.env`, `GET /api/me`, `POST /api/practice` | Plomería + Mock/QA | Práctica completa terminada; `ms` máximo < 2 000 |
| 11 | Registro `PUT /api/me/submission`; bot reiniciado desde el commit congelado | PO | `GET /api/me` muestra el hash correcto |

## 8. Riesgos

| Riesgo | Probabilidad | Mitigación |
|---|---|---|
| Nadie sabe Go y algo traba al equipo | Media | Spike con límite de 30 min; solo librería estándar; plomería probada hoy, no mañana |
| Cambios en el OpenAPI definitivo | Alta | Parseo aislado en `parse.go` con `testdata/`; 15 min reservados a las 8:00 |
| Timeout por red | Media | Presupuesto 1 500 ms, warm-up, calibración con `ms` reales |
| Bot caído > 30 s | Media | Supervisor, `recover` en el loop, timeout 35 s en el cliente, segunda laptop (apagada hasta hacer falta) |
| Condición de carrera entre árbitro y app | Media | Motor sin estado global, mutex en `api`, tests con `-race` |
| Filas/columnas invertidas al interpretar `board` | Alta (error típico de la IA) | Test con tablero dibujado; comparación `board` vs reconstrucción desde `moves` |
| Jugada ilegal por bug | Baja | Guard contra `legal_moves` + test de 1 000 posiciones |
| Bitboard/TT con bugs sutiles | Media | Fase 2, apagables, self-play contra la base |
| Ejemplo `turn` del OpenAPI incoherente (board con 6 fichas vs 8 `moves`; `you` no coincide con la paridad) | Detectado | `testdata` lo usa solo para parseo; la reconstrucción se prueba con partidas generadas por el mock; consultar en el grupo |

## 9. Cómo se valida

- Comando único desde un clon limpio: todo en verde, `-race` limpio, cobertura ≥ 70 % en `game` y `engine` (covercheck falla si baja).
- Self-play contra el mock con latencia: cero errores, cero jugadas > 1 800 ms.
- Práctica real el jueves contra `casa-basico` y `casa-aleatorio`, como jugador 1 y como 2.
- Revisión cruzada: nadie integra código de la IA sin que otro lo lea.

## 10. Qué se delega a la IA y qué se verifica a mano

| Se delega | Se verifica a mano |
|---|---|
| Scaffolding del módulo, `cmd/covercheck`, `httptest` de los tests | Orientación del tablero con un test dibujado |
| Mock a partir del OpenAPI | Que el mock devuelve exactamente los `testdata` (nulls, 409, 400) |
| Ventanas precalculadas, esqueleto del negamax | Tiempo medido en una partida real; que no haya `time.Now()` por nodo ni asignaciones en la búsqueda (`go test -bench` + `-benchmem`) |
| Tests table-driven desde la especificación | Que prueban comportamiento con tableros concretos |
| Borradores de docs y explicaciones de Go para el equipo | Que describen el código final; que cada uno entiende lo que explica |

Todo lo que la IA devuelva mal se anota en `docs/bitacora-ia.md` en el momento (hora, quién, qué, cómo se detectó).
