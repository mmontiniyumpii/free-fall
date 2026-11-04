# Cuatro en línea 8×7 — Equipo 3

Bot de Cuatro en línea para un tablero de 8 columnas por 7 filas, y una aplicación web para jugar contra él. Se hizo para el challenge de IA aplicada al desarrollo de Yompi, Yalutec y Migo, del 5 de noviembre de 2026.

- **Bot** (`bot/`): Go, solo con la librería estándar. Juega en el torneo contra el árbitro por long polling y además expone una API local para la app. El torneo y la app usan el mismo motor: negamax con poda alfa-beta e iterative deepening.
- **App** (`web/`): Angular. Renderiza el estado que devuelve el bot y no implementa reglas del juego.

## Requisitos

- **Go 1.27** o superior. Se probó con 1.27.2.
- **Node** en una versión **LTS**, con npm. Con Node 25 funciona, pero Angular muestra una advertencia.
- **En Windows:** `go test -race` necesita cgo y un compilador de C, por ejemplo gcc de MinGW-w64, en el `PATH`.

## Pruebas (comando único)

```bash
npm install   # instala también web/ (postinstall)
npm test
```

`npm test` corre dos suites en orden y termina con error si falla cualquiera de las dos:

1. **`test:bot`**: los tests de Go con `-race` y reporte de cobertura por paquete. Después corre `covercheck`, que falla si `internal/game` o `internal/engine` quedan por debajo del **70 %**.
2. **`test:web`**: los 152 tests de Angular, con Vitest.

Se puede correr una sola de las suites con `npm run test:bot` o `npm run test:web`.

## Bot

```bash
cd bot && go run ./cmd/bot
```

Si `ARBITRO_URL` no está definida, el bot no se conecta al árbitro y solo levanta la API local en `http://localhost:3000`, que es la que usa la app. El contrato de esa API está en `docs/api-local.md`.

Si `ARBITRO_URL` está definida, además juega en el árbitro. Antes de su primera jugada el motor hace un warm-up de ~1 s.

### Variables de entorno

Para configurarlas, copiá `.env.example` a `.env` y completá los valores. El token del equipo **nunca** se commitea.

| Variable | Por defecto | Para qué sirve |
|---|---|---|
| `ARBITRO_URL` | vacía | URL del árbitro (o del mock, `http://localhost:4000`). Vacía = solo API local |
| `TEAM_TOKEN` | vacía | Token del equipo. Es obligatorio si `ARBITRO_URL` está definida. No aparece en ningún log |
| `THINK_MS` | `1500` | Presupuesto de cómputo por jugada. La búsqueda se corta en `THINK_MS + 250 ms` |
| `PORT` | `3000` | Puerto de la API local |
| `POLL_WAIT` | `20` | Segundos de long poll a `GET /api/next` (entre 0 y 25) |
| `TT_ENABLED` | `false` | Reservada para la tabla de transposición (fase 2). Hoy no tiene efecto |
| `MOCK_LATENCY_MS` | `0` | Solo para el mock: demora artificial en cada respuesta |

Si un valor es inválido, el bot no arranca y el mensaje de error dice qué variable revisar.

### En el torneo: con supervisor

```bash
scripts/run-bot.sh                                        # macOS / Linux
powershell -ExecutionPolicy Bypass -File scripts\run-bot.ps1   # Windows
```

El supervisor compila `bot/bin/bot`, carga `.env` si existe y vuelve a lanzar el bot cada vez que se cae. Con `kill -9` el bot vuelve a responder en ~2,3 s. Ctrl+C detiene el bot y el supervisor.

**La demo de la app se hace con otra instancia del bot**, sin `ARBITRO_URL`, para que nadie juegue en la instancia conectada al árbitro. La app busca la API en el puerto 3000. Por eso, en el `.env` de la instancia del torneo hay que poner otro puerto, por ejemplo `PORT=3001`, y la demo se levanta aparte con `cd bot && go run ./cmd/bot`.

## App web

```bash
npm run dev:web                  # http://localhost:4200
npm --prefix web run start:lan   # para abrirla desde el teléfono u otra máquina de la red
```

La app necesita el bot corriendo en el puerto 3000. Cómo se juega está en el manual de usuario (`docs/MANUAL.md`).

**Reproducir partidas del torneo:** exportá `ARBITRO_URL` antes de `npm run dev:web`. El servidor de desarrollo hace de proxy hacia el árbitro (`web/proxy.conf.mjs`). Sin la variable, apunta al mock en `:4000`.

## Desarrollo sin el árbitro real

**Mock del árbitro**, fiel a `docs/bases/arbitro-openapi.json`, en `http://localhost:4000`:

```bash
cd bot && go run ./cmd/mock                       # -addr :4000, -seed N para repetir partidas
cd bot && MOCK_LATENCY_MS=300 go run ./cmd/mock   # con latencia de red simulada
```

Cualquier token `Bearer` que no esté vacío es un equipo. Por ejemplo, para crear una práctica contra el bot de la casa:

```bash
curl -H 'Authorization: Bearer mi-token' -X POST localhost:4000/api/practice \
  -d '{"opponent":"casa-basico","first":1}'
```

Después se levanta el bot con `ARBITRO_URL=http://localhost:4000 TEAM_TOKEN=mi-token`. El mock agrega `POST /mock/friendly`, que **no** es parte del OpenAPI, para que `cmd/selfplay` pueda enfrentar a dos equipos.

**Self-play**: dos motores juegan entre sí a través del mock, con el mismo cliente y el mismo loop que el bot del torneo.

```bash
cd bot && go run -race ./cmd/selfplay -duration 30m -latency 300ms
```

Sale con error si hubo logs de error, partidas perdidas por tiempo, por jugada ilegal o por ausencia, o jugadas más lentas que 1 800 ms + la latencia simulada.

## Estructura

```
bot/                 módulo Go
  cmd/bot/           bot: warm-up, loop del árbitro y API local
  cmd/mock/          árbitro simulado
  cmd/selfplay/      dos motores contra el mock
  cmd/covercheck/    umbral de cobertura de game y engine
  internal/game/     reglas puras
  internal/engine/   búsqueda y evaluación
  internal/arbiter/  cliente del árbitro, parseo y loop
  internal/api/      API local para la app
  internal/config/   variables de entorno
web/                 aplicación Angular
scripts/             supervisor (run-bot.sh, run-bot.ps1)
docs/                documentación técnica, manual, bitácora de IA, contrato de la API, bases
```

## Documentación

| Documento | Qué tiene |
|---|---|
| `PLAN_BACKEND.md`, `PLAN_FRONTEND.md` | Planes de ejecución, escritos antes del código |
| `docs/TECNICO.md` | Documentación técnica y desviaciones del plan: D1–D5 del bot, D6–D11 del front |
| `docs/MANUAL.md` | Manual de usuario de la app, con capturas |
| `docs/api-local.md` | Contrato entre el bot y la app |
| `docs/bitacora-ia.md` | Qué le pedimos a la IA, qué falló y cómo lo detectamos |
| `AGENTS.md` | Reglas para los agentes de IA en este repositorio |
