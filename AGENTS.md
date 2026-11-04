# AGENTS.md — Reglas para agentes de IA en este repositorio

Este archivo lo leen Claude Code y cualquier otro agente que trabaje en el repo. Es también el `CLAUDE.md` del proyecto (enlazado o copiado).

## Qué es este proyecto

Bot de Cuatro en línea en tablero **8 columnas × 7 filas** para el challenge interno de Yompi (5 nov 2026), más una aplicación web para jugar contra él y un mock del árbitro para desarrollar sin conexión. Las bases están en `docs/bases/` (PDF y `arbitro-openapi.json`). Los planes están en `PLAN_BACKEND.md` y `PLAN_FRONTEND.md`: **leelos antes de proponer arquitectura**; no se reescriben, las desviaciones se anotan en `docs/TECNICO.md`.

El backend está en **Go** (librería estándar únicamente) y el frontend en **Angular**. Nadie del equipo tiene experiencia previa en Go: cuando generes código Go, explicá en una línea cualquier construcción no obvia (punteros, `defer`, canales, `select`) para que el equipo pueda defenderla ante el jurado.

## Estructura

```
bot/                     módulo Go (un solo go.mod)
  cmd/bot/               main: loop del árbitro + API local + warm-up
  cmd/mock/              árbitro simulado fiel al OpenAPI
  cmd/selfplay/          dos motores contra el mock
  cmd/covercheck/        falla si la cobertura de game+engine < 70 %
  internal/game/         reglas puras (sin HTTP, sin motor)
  internal/engine/       búsqueda y evaluación (sin HTTP)
  internal/arbiter/      cliente del árbitro, parseo, loop; testdata/ con ejemplos del OpenAPI
  internal/api/          API HTTP local para la app web
  internal/config/       variables de entorno
web/                     aplicación Angular
docs/                    TECNICO.md, MANUAL.md, bitacora-ia.md, capturas/, bases/
scripts/                 run-bot.sh / run-bot.ps1 (supervisor)
```

## Invariantes del dominio (no negociables)

- `board[fila][columna]`, **7 filas × 8 columnas, fila 0 es la de abajo**. 0 vacío, 1 abrió la partida, 2 respondió. Es exactamente el formato del árbitro y del `GameState`; cualquier representación interna distinta vive solo dentro de `internal/engine`.
- Una jugada es una columna 0–7; la ficha cae a la fila vacía más baja.
- Victoria: 4 consecutivas en horizontal, vertical o cualquier diagonal. Empate: 56 casillas llenas sin ganador.
- El motor **siempre** devuelve una columna incluida en `legal_moves`; hay un guard final y un test que lo cubre.
- Presupuesto de cómputo `THINK_MS` (1 500 ms por defecto); el límite del árbitro es 3 000 ms contados desde su reloj. Nunca calcular tiempo a partir de `served_at`.
- `match_id` y `last_move` pueden ser `null`: en los structs son punteros. Campos desconocidos se ignoran (comportamiento por defecto de `encoding/json`); no usar `DisallowUnknownFields`.
- `GET /api/next` puede traer turnos de distintas partidas: siempre usar el `game_id` de la respuesta.

## Invariantes técnicos de Go (trampas conocidas)

- `http.Client` **siempre con `Timeout`** (35 s para el long poll de 20 s; 5 s para `POST /api/move`). Sin timeout, un socket colgado deja al bot mudo y se pierde la partida por ausencia.
- El motor es **reentrante**: sin variables de paquete mutables; cada `ChooseMove` crea su propia posición. Las goroutines del árbitro y de la app lo llaman en paralelo.
- Sin asignaciones dentro de la búsqueda: arrays fijos (`[56]uint8`, `[8]uint8`), `drop`/`undo` in place, slices preasignadas. Verificar con `go test -bench . -benchmem` en `internal/engine` (allocs/op debe ser 0 en el nodo).
- `time.Now()` solo cada 2 048 nodos, nunca por nodo.
- `internal/api` guarda las partidas en un mapa protegido por `sync.Mutex`. Todo test se corre con `-race`.
- JSON con tags explícitos en snake_case (`json:"game_id"`); no confiar en los nombres por defecto.
- Errores: siempre se devuelven y se manejan; nada de `_ = err`. El loop del árbitro envuelve cada iteración con `recover` y duerme 1 s tras un error; tras `idle` vuelve a llamar sin dormir.
- Logs con `log/slog` en una línea por turno: `game_id`, `column`, `depth`, `nodes`, `elapsed_ms`. Nunca loguear el token ni los headers.

## Comandos

```
npm test                 # ÚNICO comando de pruebas (raíz): go test ./... -race -cover + covercheck
go test ./... -cover     # equivalente dentro de bot/
go run ./cmd/bot         # bot (loop del árbitro + API local) con ARBITRO_URL del entorno
go run ./cmd/mock        # árbitro simulado en http://localhost:4000
go run ./cmd/selfplay    # dos motores contra el mock, con latencia artificial
go build -o bin/bot ./cmd/bot   # binario para el supervisor
npm run dev:web          # app Angular en http://localhost:4200
scripts/run-bot.sh       # bot bajo supervisor con reinicio automático (run-bot.ps1 en Windows)
```

Variables de entorno (ver `.env.example`): `ARBITRO_URL`, `TEAM_TOKEN`, `THINK_MS`, `PORT`, `POLL_WAIT`, `TT_ENABLED`, `MOCK_LATENCY_MS`.

## Convenciones

- `gofmt` y `go vet` limpios antes de cada commit. Paquetes chicos con una responsabilidad; nombres en inglés en código, comentarios y docs en español.
- `internal/game` e `internal/engine` no importan `net/http` ni `internal/arbiter`/`internal/api`. Deben poder probarse sin levantar servidores.
- Tests table-driven con el paquete `testing` estándar; sin frameworks de aserciones. Los tests prueban **comportamiento** (tableros concretos, resultados esperados), no si una función fue llamada. Los fixtures del árbitro salen de los ejemplos del OpenAPI, viven en `internal/arbiter/testdata/` y los usa también el mock.
- El cliente del árbitro se prueba contra `httptest.NewServer`; ningún test toca la red.
- Umbral de cobertura 70 % sobre `internal/game` e `internal/engine`, verificado por `cmd/covercheck`; no bajarlo.
- Commits chicos y frecuentes a `main`, mensaje en español en imperativo (`agrega detección de diagonales`). Cada uno commitea en su carpeta; `internal/game/types.go` y el shape de `GameState` solo cambian avisando al equipo.

## Qué NO hacer

- No escribir ni loguear el token. No tocar `.env`. No commitear `.env`, `bin/`, `coverage.out`, `node_modules`, `dist`.
- No modificar `PLAN_BACKEND.md` ni `PLAN_FRONTEND.md` después del commit inicial.
- No agregar dependencias a `go.mod`: librería estándar únicamente. Si parece imprescindible, pedir confirmación a un humano.
- No duplicar reglas del juego en `web/`: el frontend renderiza el `GameState` que devuelve la API.
- No cambiar `internal/game/types.go` ni el shape de `GameState` sin que lo pida explícitamente un humano.
- No introducir bitboards ni tabla de transposición fuera de la fase 2 ni sin flag para apagarlos.
- No "arreglar" un test para que pase: si un test falla, primero decidir si el bug está en el código o en el test, y anotarlo en la bitácora.
- No usar goroutines dentro de la búsqueda (paralelizar alfa-beta no es trivial y no se puede explicar en 10 minutos).

## Bitácora de IA

Cada vez que un agente devuelve algo incorrecto, incompleto o inventado (campo del OpenAPI que no existe, filas invertidas, test que no prueba nada, `http.Client` sin timeout, estimación de tiempo falsa), se agrega una línea a `docs/bitacora-ia.md`:

```
| hora | quién | herramienta | qué se pidió | qué falló | cómo se detectó | decisión |
```

Esta bitácora se usa en la exposición y en `docs/TECNICO.md`.

## Al terminar una tarea

1. `npm test` en verde, `-race` limpio, cobertura visible.
2. Si cambió comportamiento, actualizar `docs/TECNICO.md` (y `README.md` si cambió un comando o variable).
3. Resumir en el commit qué se delegó a la IA y qué se verificó a mano.
