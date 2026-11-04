# PLAN_FRONTEND.md — Aplicación web de Cuatro en línea 8×7

> Equipo 3. Se escribe antes del código. Las desviaciones se explican en `docs/TECNICO.md`.

## 1. Objetivo y alcance

Una página donde una persona juega contra el bot del equipo, en la misma computadora o en la red local. El frontend **no implementa reglas**: envía la columna elegida a la API local del bot y renderiza el `GameState` que vuelve. Así el algoritmo del torneo y el de la app son el mismo código.

**Obligatorio**
- Tablero 8×7 interactivo (fila 0 abajo), con indicación clara de a quién le toca y qué columnas están disponibles.
- Elegir quién abre (humano o bot), reiniciar, mostrar resultado: ganador con la línea de cuatro resaltada, o empate.
- Limpia, consistente, usable en pantalla de laptop. Sin errores en consola.

**Opcional (en este orden, solo si sobra tiempo)**
1. Historial de jugadas en pantalla.
2. Responsive para teléfono (puntos extra explícitos).
3. Deshacer (requiere endpoint nuevo: `POST /local/games/:id/undo`).
4. Reproducir una partida del árbitro desde `GET /api/games/{id}` (público, sin token).

## 2. Stack

Angular (standalone components, signals para estado local, `HttpClient`, TypeScript estricto). Sin librerías de UI; estilos propios con variables CSS. Iconos: Phosphor.

Plan B si el tooling de Angular consume más de 30 minutos: Vite + TypeScript vanilla con la misma estructura de componentes.

## 3. Pantallas y componentes

Una sola pantalla.

```
┌──────────────────────────────────────────────────────┐
│  Cuatro en línea 8×7          [Humano abre ▾] [Nueva] │  ← app-toolbar
├──────────────────────────────────────────────────────┤
│  ● Te toca a vos   |  profundidad 9 · 1 312 ms        │  ← app-status
├──────────────────────────────────────────────────────┤
│   ▼   ▼   ▼   ▼   ▼   ▼   ▼   ▼   (columnas legales)  │
│  ○ ○ ○ ○ ○ ○ ○ ○                                      │
│  ○ ○ ○ ○ ○ ○ ○ ○                                      │  ← app-board (7 filas × 8 columnas,
│  ○ ○ ○ ● ○ ○ ○ ○                                      │     fila 0 se dibuja abajo)
│  ...                                                   │
├──────────────────────────────────────────────────────┤
│  Historial: 3, 2, 3, 4 …                              │  ← app-history (opcional)
└──────────────────────────────────────────────────────┘
```

| Componente | Responsabilidad | Inputs / outputs |
|---|---|---|
| `AppComponent` | Orquesta; mantiene `GameState` en un signal | — |
| `BoardComponent` | Dibuja el tablero, resalta columnas legales y la línea ganadora, emite la columna clickeada | `state: GameState`, `disabled: boolean` → `columnSelected` |
| `StatusComponent` | Turno actual, "pensando…", resultado, datos del bot | `state` |
| `ToolbarComponent` | Quién abre, nueva partida | `firstPlayer` → `newGame` |
| `HistoryComponent` (opcional) | Lista de jugadas | `history: Column[]` |
| `GameApiService` | Llama a la API local | `create(first)`, `move(id, col)`, `get(id)` |

## 4. Estado y flujo

```
inicio ──► [Nueva partida] ──► POST /local/games {first}
   │                                   │
   ▼                                   ▼
 esperando humano ◄──── GameState ◄── (si abre el bot, el backend ya responde con su jugada)
   │ click en columna legal
   ▼
 pensando (tablero bloqueado, spinner) ──► POST /local/games/:id/moves {column}
   │                                            │
   └────────────── GameState ◄──────────────────┘
          status = playing → esperando humano
          status = won     → resaltar winning_cells, mostrar ganador
          status = draw    → mostrar empate
```

Estados visibles que el jurado va a buscar: turno del humano / turno del bot / pensando / jugada ilegal (columna llena: la UI no deja clickear, y si el backend devuelve error se muestra un mensaje) / fin de partida / error de conexión con el bot.

## 5. Comunicación con el bot

Base URL configurable (`environment.apiUrl`, por defecto `http://localhost:3000`). Contrato (`GameState`) definido en `PLAN_BACKEND.md`, sección 4. El frontend tipa la respuesta y no infiere nada que no venga del backend (ganador, línea, columnas legales).

Durante el desarrollo, mientras la API no existe, se trabaja con un `GameApiService` falso que devuelve un `GameState` fijo.

## 6. Pasos en orden, con criterio de listo

| # | Paso | Listo cuando |
|---|---|---|
| 1 | Scaffold Angular, estructura de componentes, `GameState` tipado, servicio falso | La app arranca y muestra un tablero con datos fijos |
| 2 | `BoardComponent`: grilla 8×7, fila 0 abajo, columnas legales marcadas, click emite columna | Click en una columna llena no hace nada; la ficha aparece abajo |
| 3 | `StatusComponent` + `ToolbarComponent`: turno, nueva partida, quién abre | Se puede empezar partida como humano o como bot |
| 4 | Conexión con la API real; estado "pensando" bloquea el tablero | Partida completa contra el bot de punta a punta |
| 5 | Fin de partida: línea ganadora resaltada, empate, botón de reiniciar visible | Ganar, perder y empatar (tablero forzado) se ven bien |
| 6 | Pulido visual: colores consistentes, animación de caída simple, sin errores en consola | Revisión de otro integrante sin explicación: entiende todo solo |
| 7 | Opcional: historial, responsive, deshacer, reproducir partida | Cada uno con su propio check |
| 8 | Capturas para `docs/MANUAL.md` (jueves, con el árbitro real de fondo si se reproduce una partida) | Manual con 4–6 capturas anotadas |

## 7. Riesgos

| Riesgo | Mitigación |
|---|---|
| Tooling de Angular (build/test) consume tiempo | Límite de 30 min; plan B Vite vanilla |
| Tests del front rompen el "pasan todas" del comando único | Tests mínimos y estables; `npm test` (raíz) también corre los del front. Si uno es frágil se arregla o se elimina, nunca queda afuera del comando único |
| Dibujar la fila 0 arriba en vez de abajo | `BoardComponent` invierte el orden de filas solo al renderizar; test del componente lo verifica |
| CORS entre 4200 y 3000 | CORS habilitado en la API local; opción de servir `web/dist` desde el bot en el mismo puerto para la demo |
| Demo falla por el bot caído | La app muestra "no se pudo conectar con el bot" en vez de un error en consola |

## 8. Cómo se valida

- Partida completa como humano abriendo y como bot abriendo.
- Caso de empate forzado (tablero casi lleno vía API o estado de prueba).
- Alguien que no tocó el frontend juega siguiendo solo `docs/MANUAL.md`.
- Consola sin errores ni warnings durante una partida.
- `ng build` sin errores; Lighthouse/responsive opcional.

## 9. Qué se delega a la IA y qué se verifica a mano

- Se delega: scaffolding, componentes base, estilos iniciales, servicio HTTP, borrador del manual.
- Se verifica a mano: orientación del tablero, que la UI nunca permita columna llena, estados de fin de partida, que no haya lógica de reglas duplicada en el front, legibilidad del manual para alguien no técnico.

## 10. Responsable

Matias (frontend, README, `docs/MANUAL.md`, demo de la exposición).
