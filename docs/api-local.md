# Contrato: API local del bot para la aplicación web

Este documento es el contrato entre `bot/internal/api` (Go) y `web/` (Angular). **Ningún agente lo modifica**; los cambios los decide una persona y se anuncian al equipo. El backend es la única fuente de verdad sobre reglas, legalidad, ganador y línea de cuatro: el frontend solo renderiza lo que recibe.

## Base y transporte

- Base URL: `http://localhost:3000` (variable `PORT` en el bot; `environment.apiUrl` en el front).
- JSON en snake_case en ambos sentidos. `Content-Type: application/json`.
- CORS habilitado para cualquier origen en `GET`, `POST`, `OPTIONS` (la app corre en `:4200` en desarrollo).
- Sin autenticación: la API local solo escucha en la máquina del equipo.
- Las partidas viven en memoria del proceso del bot; se pierden al reiniciarlo.

## Endpoints

### `GET /health`

Para que el front sepa si el bot está vivo antes de mostrar el tablero.

```json
200  { "ok": true, "arbiter_connected": false, "think_ms": 1500 }
```

### `POST /local/games` — nueva partida

```json
Request   { "first": "human" }          // "human" | "bot"
Response  201 GameState
```

- `first: "human"` → `human = 1`, `bot = 2`, tablero vacío, `to_move = 1`.
- `first: "bot"` → `human = 2`, `bot = 1`, y la respuesta **ya incluye la jugada de apertura del bot**: tablero con una ficha, `history` de longitud 1, `last_bot` informado, `to_move = 2`.

### `POST /local/games/{id}/moves` — jugada del humano

```json
Request   { "column": 3 }               // 0..7
Response  200 GameState
```

Aplica la jugada del humano; si la partida sigue, el bot responde en la misma llamada. La respuesta refleja **ambas** jugadas. Puede tardar hasta `think_ms` + ~100 ms: el front muestra "pensando…" y bloquea el tablero mientras espera.

Si la jugada del humano cierra la partida (gana o empata), la respuesta trae el estado final sin jugada del bot.

Errores:

| Código | Cuerpo | Cuándo |
|---|---|---|
| 400 | `{ "ok": false, "error": "illegal_move", "message": "La columna 3 está llena." }` | Columna llena o fuera de rango |
| 400 | `{ "ok": false, "error": "validation", "message": "..." }` | Body inválido |
| 404 | `{ "ok": false, "error": "unknown_game", "message": "..." }` | `id` inexistente |
| 409 | `{ "ok": false, "error": "game_over", "message": "..." }` | La partida ya terminó |
| 409 | `{ "ok": false, "error": "not_your_turn", "message": "..." }` | No es el turno del humano (no debería pasar; defensa) |

El formato de error es el mismo que usa el árbitro, a propósito.

### `GET /local/games/{id}` — estado actual

```json
200 GameState   |   404 { "ok": false, "error": "unknown_game" }
```

### `POST /local/games/{id}/undo` — opcional (fase opcional del front)

Deshace la última jugada del humano **y** la respuesta del bot. Responde `200 GameState` o `409 { "error": "nothing_to_undo" }`. Solo se implementa si sobra tiempo en ambos lados.

## `GameState`

```json
{
  "id": "l_0001",
  "board": [
    [0, 0, 0, 1, 2, 0, 0, 0],
    [0, 0, 0, 1, 2, 0, 0, 0],
    [0, 0, 0, 1, 0, 0, 0, 0],
    [0, 0, 0, 2, 0, 0, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0]
  ],
  "to_move": 1,
  "legal_moves": [0, 1, 2, 3, 4, 5, 6, 7],
  "status": "playing",
  "winner": null,
  "winning_cells": null,
  "history": [3, 4, 3, 4, 3, 3],
  "human": 1,
  "bot": 2,
  "last_bot": { "column": 3, "depth": 9, "nodes": 412380, "elapsed_ms": 1290 }
}
```

| Campo | Tipo | Significado |
|---|---|---|
| `id` | string | Identificador de la partida |
| `board` | `number[7][8]` | `board[fila][columna]`, **fila 0 es la de abajo**. 0 vacío, 1 jugador que abrió, 2 el otro. Mismo formato que el árbitro. |
| `to_move` | `1 \| 2 \| null` | A quién le toca; `null` si la partida terminó |
| `legal_moves` | `number[]` | Columnas no llenas, ascendente. Vacío si terminó. |
| `status` | `"playing" \| "won" \| "draw"` | Estado |
| `winner` | `1 \| 2 \| null` | Solo con `status = "won"` |
| `winning_cells` | `[fila, columna][] \| null` | Exactamente 4 celdas cuando `status = "won"`; `null` en otro caso |
| `history` | `number[]` | Columnas jugadas desde el inicio, en orden (misma semántica que `moves` del árbitro) |
| `human` / `bot` | `1 \| 2` | Qué número tiene cada uno en esta partida |
| `last_bot` | objeto \| null | Última jugada del bot: columna, profundidad alcanzada, nodos visitados, ms de cómputo. `null` si el bot aún no jugó. |

Para saber si le toca al humano: `status === "playing" && to_move === human`.

### Ejemplo de partida ganada por el humano

```json
{ "id": "l_0002", "board": "...", "to_move": null, "legal_moves": [],
  "status": "won", "winner": 1, "winning_cells": [[0, 2], [1, 3], [2, 4], [3, 5]],
  "history": [2, 3, 3, 4, 5, 4, 4, 5, 6, 5, 5], "human": 1, "bot": 2,
  "last_bot": { "column": 5, "depth": 10, "nodes": 980112, "elapsed_ms": 1402 } }
```

### Ejemplo de empate

```json
{ "status": "draw", "winner": null, "winning_cells": null, "to_move": null, "legal_moves": [], "...": "..." }
```

## Reglas para ambos lados

- El backend garantiza que `legal_moves`, `status`, `winner` y `winning_cells` son consistentes con `board`; hay un test que compara la respuesta de `POST /local/games` con `docs/fixtures/game-state.example.json` (misma forma y tipos).
- El frontend no calcula legalidad ni victoria. Deshabilita columnas que no estén en `legal_moves`, resalta `winning_cells`, y muestra `last_bot` como información.
- Campos nuevos que el backend agregue en el futuro no rompen al front (ignorar desconocidos). Campos existentes no cambian de nombre ni de tipo sin cambiar este documento.
