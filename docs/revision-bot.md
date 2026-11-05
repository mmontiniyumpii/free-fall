# Revisión técnica del bot (4 nov 2026, noche previa al torneo)

Revisión hecha con Claude Code (Opus 5.5) a pedido de Matias. Objetivo: que el bot gane todo lo que se pueda ganar y no pierda ninguna partida por error propio. **No se modificó código del bot**: solo se agregaron archivos de prueba sin commitear (ver "Archivos de la revisión" al final).

Leyenda: ✅ cumple · ⚠️ cumple con reservas o riesgo bajo · ❌ nos puede hacer perder partidas.

## Resumen

| # | Hallazgo | Impacto | Estado | Esfuerzo del fix |
|---|---|---|---|---|
| 1 | Si el `POST /api/move` falla por red una sola vez, la jugada llega a los 4,5 s y se pierde por timeout | Pierde la partida | ❌ | 1 h |
| 2 | El motor pierde finales largos por zugzwang (paridad) que no ve: 5 derrotas en 20 partidas contra el mismo motor limitado a profundidad 4 | Pierde partidas contra bots parecidos al nuestro | ❌ | 2 h (poda de jugadas perdedoras) · 3-4 h (tabla de transposición) |
| 3 | Una segunda instancia del bot en la misma máquina habla con el árbitro antes de fallar por el puerto ocupado, y el supervisor la relanza cada ~2 s | Puede llevarse un turno y perderlo | ⚠️ | 20 min |
| 4 | La evaluación no es simétrica entre los dos jugadores | Juega algo peor | ⚠️ | 2 h con self-play A/B |
| 5 | Dentro del árbol, el orden de jugadas es solo "centro primero": no prueba antes las que ganan o bloquean | Menos profundidad | ⚠️ | incluido en el fix 2 |
| 6 | `TT_ENABLED` se lee pero no hace nada; el PDF de las bases no está en `docs/bases/` | Calidad / documentación | ⚠️ | 15 min |

Todo lo demás (reglas, guard, presupuesto, reentrancia, parseo, logs, juego limpio, cobertura) está en verde. El detalle de cada punto y su evidencia están abajo.

## Cómo se midió

- `npm test` (raíz): verde. Bot: todos los paquetes `ok` con `-race`; `internal/game` 100 %, `internal/engine` 97 %; covercheck `ok`. Front: 17 archivos, 162 tests en verde.
- Desde un clon limpio (`git clone` + `npm run test:bot`): verde, mismas coberturas.
- `go test ./... -race -cover -bench . -benchmem`: verde. Sin `-race`: `BenchmarkSearchDepth8` = 13,1 ms/op, 36 983 nodos, **0 B/op, 0 allocs/op**.
- Equipo: Apple M4, Go 1.27.2. Las partidas en memoria corren 4 a la vez, así que la profundidad que se mide ahí es un poco menor que con el motor solo.

---

## 1. Lo que nos hace perder partidas

### ❌ 1.1 Un `POST /api/move` cortado por la red hace perder la partida por timeout

**Evidencia.** `internal/arbiter/loop.go:86-98`: si `client.Move` devuelve un error de transporte (no un 400 ni un 409), `runOnce` devuelve el error y `RunLoop` duerme `retryDelay` = 1 s (`loop.go:38`, `loop.go:53`). En la vuelta siguiente, `GET /api/next` trae **el mismo turno** (sigue pendiente) y el motor **vuelve a pensar** otros 1 750 ms.

**Escenario concreto** (test `TestAuditLoopSurvivesHostileArbiter`, posición real de medio juego con 16 fichas):

```
27.10 s  next: turno g_audit                     ← empieza el reloj del árbitro
28.85 s  move #1: conexión cortada (columna 2)   ← el motor pensó 1 750 ms
29.85 s  next: turno g_audit                     ← 1 s de espera tras el error
31.60 s  move #2: aceptada (columna 2)           ← volvió a pensar 1 750 ms
total desde la primera entrega: 4,504 s > 3 s    ← el árbitro real responde 400 timeout: partida perdida
```

Basta un corte de wifi de un segundo en el salón, o un reset de conexión del proxy del árbitro.

**Sobre el criterio de "reintentar solo si NO se envió".** Revisado contra el OpenAPI 1.0.1: un `POST /api/move` para un turno que ya no está pendiente responde **409 `not_your_turn`, "No penaliza"** (`docs/bases/arbitro-openapi.json`, `/api/move`, respuesta 409). Reenviar la **misma** columna es seguro: si la primera llegó, la segunda recibe 409 y no pasa nada. El riesgo real es lo contrario: **no** reintentar, o reintentar tarde. Además, el timeout del cliente de jugadas es de 5 s (`client.go:38`), más que el plazo entero del árbitro (3 s): si el POST se cuelga, se pierde igual.

**Fix propuesto (1 h).**
1. En `runOnce`, si `Move` falla por transporte, reintentar **de inmediato la misma columna** mientras no pasen ~2 600 ms desde la recepción del turno (1 o 2 reintentos en la práctica).
2. Dar al POST un contexto con el tiempo que queda hasta ese límite, en vez de los 5 s fijos.
3. Recordar la última decisión (`game_id` + número de turno → columna): si el árbitro vuelve a entregar ese turno, se juega la columna guardada sin volver a pensar.
4. Test: el mismo `TestAuditLoopSurvivesHostileArbiter`, que hoy falla, pasa a la suite normal y tiene que dar < 3 s.

### ❌ 1.2 El motor pierde finales largos por zugzwang: 5 derrotas contra el mismo motor a profundidad 4

**Resultado del self-play** (`TestAuditSelfPlayDepth4`): bot hard (presupuesto 1 500 ms, sin límite de profundidad) contra el mismo motor con `MaxDepth = 4`. Fueron 20 partidas: 10 aperturas al azar de 2 jugadas, cada una jugada con los dos colores. Resultado: **15 ganadas, 0 empates, 5 perdidas**. Profundidad media del hard: 10,2.

| Apertura | Hard como jugador 1 | Hard como jugador 2 |
|---|---|---|
| [2 7] (salió dos veces) | P en 46 · P en 46 | G en 32 · G en 32 |
| [6 2] | G en 27 | G en 30 |
| [7 1] | G en 27 | G en 42 |
| [3 1] | G en 31 | G en 24 |
| [0 1] | G en 35 | G en 52 |
| [1 6] | G en 41 | G en 32 |
| [7 7] | G en 25 | **P en 45** |
| [2 0] | G en 33 | **P en 51** |
| [0 3] | **P en 46** | G en 46 |

Todas las derrotas son partidas largas (45 a 51 jugadas). En cada derrota, el motor llegó al final con el tablero casi lleno y sin jugadas seguras.

**Análisis de la derrota [2 7]** (`TestAuditAnalyzeLoss`, jugada a jugada):

```
jugada 23  jugador1 (hard)  profundidad 13  score +206   ← cree que va ganando
jugada 25  jugador1 (hard)  profundidad 14  score +106   ← sigue creyendo que gana
jugada 26  jugador2         profundidad 21  score 99979  ← el mismo motor, del otro lado, ya ve victoria forzada en 21 plies
jugada 46  gana el jugador 2
```

**No es un bug.** Después de la jugada 25 la partida ya estaba perdida, pero la derrota estaba a 21 plies y el motor solo llegaba a 14. La heurística cuenta tríos y pares en ventanas sin saber **quién va a poder ocupar cada casilla** cuando se llene el tablero. Ese es el problema clásico de la paridad (zugzwang): el que tiene la amenaza "en la fila correcta" gana el final aunque el puntaje diga lo contrario.

**La paridad en 8×7 es al revés que en el Connect 4 clásico (verificado a mano).** Supongamos una amenaza en la fila `r` (0 = abajo) y el resto del tablero lleno. Antes de esa casilla se juegan 49 + r fichas, así que esa casilla es la jugada número 50 + r. Es impar (le toca al jugador 1) cuando r es impar. Entonces el jugador 1 quiere amenazas en las filas 1, 3 y 5, y el jugador 2 en las filas 0, 2, 4 y 6. En el tablero clásico de 6 filas pasa lo contrario. Cualquier regla de paridad copiada de Internet (Allis, Pons) sale **invertida** si no se adapta.

**Contra quién perderíamos.** `casa-basico` y `casa-aleatorio` no llegan a esos finales: les ganamos 40 de 40. Los bots de los otros equipos probablemente sean alfa-beta parecidos al nuestro. Contra ellos, los finales largos sí van a aparecer, y ahí es donde hoy perdemos.

**Mitigación propuesta, en orden de riesgo creciente:**

| Opción | Qué es | Ganancia esperada | Riesgo | Esfuerzo |
|---|---|---|---|---|
| A. Poda de jugadas perdedoras | En cada nodo: (1) si una jugada gana ya, devolver sin explorar el resto; (2) si el rival tiene una amenaza jugable, la única jugada que no pierde es taparla; con dos amenazas, la posición está perdida; (3) no jugar debajo de una casilla ganadora del rival. Es lo que hace el solver de Pons | Más profundidad en las posiciones tácticas, que son justamente las de los finales. A estimar con medición | Bajo: no cambia la heurística y es fácil de testear con tableros dibujados | 2 h |
| B. Tabla de transposición con `TT_ENABLED` | Recordar posiciones ya evaluadas (en Connect 4 se repiten muchísimo por distinto orden de jugadas) | La mayor de las tres; es lo que permite resolver finales desde lejos | Medio: bugs sutiles. El flag permite apagarla | 3-4 h |
| C. Heurística con paridad | Bonus por amenazas en la fila "propia" según la cuenta de arriba | Ataca la causa, pero cambia los pesos | Medio: exige self-play A/B de 20 partidas o más | 2 h |

**Recomendación para hoy:** hacer A, medirlo con la suite táctica y con este mismo self-play, y decidir B según cuánto tiempo quede. Dejar C para después del torneo, salvo que A y B no alcancen.

### ⚠️ 1.3 Dos instancias del bot: la segunda toca el árbitro antes de morir

**Evidencia.** `cmd/bot/main.go:55-84`: primero hace el warm-up (1 s), después lanza `go arbiter.RunLoop(...)` y **recién después** intenta `ListenAndServe` en `PORT`. Si el puerto está ocupado (ya hay un bot corriendo), el proceso sale con código 1. Pero el loop ya pudo mandar un `GET /api/next`. `scripts/run-bot.sh:30-41` lo relanza cada ~2 s (1 s de warm-up más 1 s de espera), así que cada 2 s hay un proceso efímero haciendo long poll con nuestro token.

**Escenario.** Alguien abre una segunda terminal y corre `scripts/run-bot.sh` "por las dudas". Si el árbitro entrega un turno pendiente justo a ese proceso efímero, el turno queda servido a un proceso que se muere. El reloj de 3 s corre desde `served_at`, así que perdemos tiempo o la partida. Con otro `PORT`, en cambio, juegan las dos instancias a la vez.

No se pudo verificar si las bases prohíben explícitamente más de un proceso por token: **el PDF no está en `docs/bases/`**, solo el OpenAPI.

**Fix propuesto (20 min).** En `main.go`, abrir el puerto con `net.Listen` **antes** de arrancar el loop y servir con `srv.Serve(ln)`. Así una segunda instancia en la misma máquina falla sin haber hablado con el árbitro. Para dos máquinas no hay fix de código: es una regla operativa ("la segunda laptop, apagada"), que ya está en el plan §8.

---

## 2. Lo que nos hace jugar peor

### ⚠️ 2.1 La evaluación no es simétrica

`internal/engine/evaluate.go:6-11, 52-59`: suma +50 por trío propio, +10 por par propio y −60 por trío rival, pero **no resta nada por el par rival**. Negamax supone que el valor de una posición para un jugador es el negativo del valor para el otro: `evaluate(p, A) = −evaluate(p, B)`. Acá no se cumple. Un mismo tablero vale distinto según a quién le toque mover en la hoja, y entonces las profundidades pares e impares miden cosas distintas.

Además, la asimetría va al revés de la intuición táctica: el que mueve en la hoja valora **menos** su propio trío (+50) que el trío del rival (−60), aunque es él quien puede completar el suyo primero.

**No se propone cambiar los pesos sin datos.** Si se toca, se hace con self-play A/B de 20 partidas o más contra la versión actual, alternando quién abre, como pide la revisión.

### ⚠️ 2.2 El orden de jugadas dentro del árbol es solo "centro primero"

- Raíz: primero la mejor jugada de la iteración anterior, después del centro a los bordes (`ordering.go:13-38`) ✅.
- Red de seguridad antes de buscar: ganar en 1 y bloquear en 1 (`choose.go:62-71`) ✅.
- **Dentro del árbol** (`search.go:48`): siempre `centerFirst`. Una victoria inmediata en la columna 7 se descubre recién después de explorar enteras las otras siete. No hay killer moves ni history. Se arregla con la opción A del punto 1.2.

### ✅ 2.3 Profundidad alcanzada en 1 500 ms

Posiciones reales: dos motores a profundidad 6 juegan desde [3 4 2] y se mide en distintos momentos (`TestAuditDepth`, motor solo):

| Fichas | Profundidad | Nodos | Nodos/s | Tiempo |
|---|---|---|---|---|
| 3 (apertura) | 9 | 3,9 M | 2,2 M | 1 751 ms |
| 0 (tablero vacío) | 11 | 3,9 M | 2,3 M | 1 679 ms |
| 8 | 10 | 4,1 M | 2,4 M | 1 751 ms |
| 12 | 10 | 4,6 M | 2,6 M | 1 751 ms |
| 16 | 10 | 4,3 M | 2,5 M | 1 751 ms |
| 24 | 11 | 5,1 M | 2,9 M | 1 750 ms |
| 28 | 13 | 7,0 M | 4,0 M | 1 750 ms |
| 32 | 19 | 4,9 M | 5,5 M | 881 ms (encontró victoria forzada) |

En medio juego llega a 10, por encima del mínimo de 8. No hay `time.Now()` por nodo (`search.go:34`, cada 2 048 nodos) ni asignaciones (0 allocs/op). Casi todas las jugadas usan los 1 750 ms completos: la última iteración arranca antes de los 1 500 ms y la corta la parada dura. Eso no es un riesgo de tiempo, pero esa iteración incompleta se descarta.

### ✅ 2.4 Suite táctica (`TestAuditTactics`, presupuesto real 1 500 ms y `MaxDepth = 0`)

| Caso | Jugó | Esperado | Profundidad | Nodos | Tiempo |
|---|---|---|---|---|---|
| (a) gana en 1 (diagonal que termina en la columna 7) | 7 | 7 | 1 (red de seguridad) | 0 | 0 ms |
| (b) bloquea en 1 (columna 0) | 0 | 0 | 1 (red de seguridad) | 0 | 0 ms |
| (c) no juega debajo de la casilla ganadora del rival (columna 4) | 6 | ≠ 4 | 11 | 4,2 M | 1 750 ms |
| (d) arma una doble amenaza propia, gana en 3 | 2 | 2 o 5 | 3 (score 99 997 = gana en el ply 3) | 216 | 0 ms |
| (e) impide la doble amenaza rival | 4 | 1 o 4 | 10 | 3,6 M | 1 539 ms |
| (f) una sola columna libre (54 fichas) | 7 | 7 | 0 (no busca) | 0 | 0 ms |
| (g) a 2 fichas de llenarse | 5 | 5 o 6 | 2 (resuelto hasta el final) | 6 | 0 ms |
| (h) apertura | 3 | 3 o 4 | 11 | 3,9 M | 1 685 ms |

8 de 8 correctos.

### ✅ 2.5 Self-play contra las casas

**En memoria** (`TestAuditSelfPlayHouse`, réplica exacta de `cmd/mock/house.go`; 10 partidas abriendo y 10 respondiendo por rival):

| Rival | Resultado | Profundidad media | Pensamiento máximo |
|---|---|---|---|
| `casa-aleatorio` | 20 G / 0 E / 0 P | 9,5 | 1 751 ms |
| `casa-basico` | 20 G / 0 E / 0 P | 8,9 | 1 751 ms |

**Bot real contra el mock por HTTP** (binario `cmd/bot` + `cmd/mock`, `POST /api/practice`, 5 partidas por rival y por color): ver la sección "Resultado contra el mock" al final.

### ⚠️ 2.6 Horizonte y paridad: ¿vale la pena mitigar hoy?

Sí, pero **en profundidad, no en heurística**. El punto 1.2 muestra que las derrotas vienen de finales que el motor no llega a ver. Buscar más hondo (opciones A y B) arregla eso sin cambiar qué valora el bot. Cambiar la heurística para que entienda la paridad (opción C) ataca la causa, pero exige calibrar pesos y probarlos contra la versión actual, y no hay tiempo para hacerlo bien antes del torneo.

---

## 3. Motor: seguridad

| Punto | Estado | Evidencia |
|---|---|---|
| Guard final contra `legal_moves` en todos los caminos, incluido el fallback | ✅ | `choose.go:50-52` aplica `guard` a todo `done(...)`, también a `legal[0]` (`choose.go:55, 59`). El loop vuelve a aplicar `guard` después de `safeChoose`, que cubre un panic del motor (`loop.go:82-83, 104-123`). Tests: `TestGuardUsesGivenLegalMoves`, `TestNeverIllegal` (1 000 posiciones), `TestLoopGuardReplacesIllegalChoice`, `TestLoopRecoversFromPanicInChoose` |
| Iterative deepening devuelve la última profundidad completa | ✅ | `choose.go:85-89`: si `searchRoot` devuelve `ok = false` se queda con la anterior. Test: `TestBudgetedResultComesFromACompletedDepth` |
| ¿Y si no termina la primera iteración? | ✅ | No puede pasar: la profundidad 1 tiene ≤ 8 nodos y el reloj se mira cada 2 048. Igual, `best` arranca en `order[0]`, que es legal (`choose.go:78`) |
| Parada dura ≤ 1 800 ms desde la recepción | ✅ | Parada a `THINK_MS + 250` = 1 750 ms (`choose.go:15, 48`), medida desde `ChooseMove`, que arranca microsegundos después de recibir el turno (`loop.go:80-82`). **Medido de punta a punta** (`TestAuditReceptionToMoveUnder1800ms`): turno entregado → POST recibido = **1 751 ms**. El test normal `TestRespectsBudget` mide solo el motor; se propone sumar el de punta a punta a la suite |
| El torneo juega con `MaxDepth = 0` | ✅ | `chooser.go:21`. Test: `TestEngineChooserNeverLimitsDepth` |
| Reentrancia | ✅ | Las únicas variables de paquete son `windows`, `centerFirst` y `directions`: de solo lectura (`evaluate.go:16`, `ordering.go:8`, `rules.go:47`). `maxDepthFor` en `api/games.go:24` también es de solo lectura. Tests: `TestConcurrentCallsAreIndependent`, `TestThinkingDoesNotBlockOtherGames`, `TestParallelGames`, con `-race` limpio |
| Ventanas: 107 = 35 + 32 + 40 | ✅ | `TestWindows` (engine) y `TestAuditFindWinEveryWindowFromEveryCell` (game) |
| Victoria = valor grande − ply | ✅ | `search.go:13, 55, 86` |

## 4. Cliente del árbitro

| Punto | Estado | Evidencia |
|---|---|---|
| `http.Client` con timeout: 35 s para el long poll, 5 s para la jugada | ✅ / ⚠️ | `client.go:37-48`. Los 5 s del POST superan el plazo de 3 s del árbitro (ver 1.1) |
| `recover` en el loop | ✅ | `loop.go:63-67`, `safeChoose` en `loop.go:105-113` |
| `idle` → vuelve a llamar sin dormir; error → duerme 1 s | ✅ | `loop.go:45-56`. Test: `TestLoopDoesNotSleepAfterIdleOrNotYourTurn`, `TestLoopRetriesAfterErrorWithDelay` |
| 409 `not_your_turn` → ignorar y seguir | ✅ | `loop.go:89-95` (también 400, sin reintento) |
| `match_id` y `last_move` null, campos desconocidos | ✅ | `parse.go:42, 49`; test `TestParseNextNullsAndUnknownFields`; el turno de la revisión trae `"campo_nuevo"` |
| Turnos de varias partidas | ✅ | Usa siempre `turn.GameID` (`loop.go:86`). Test: `TestLoopPlaysEachTurnWithItsGameID` |
| `board` del árbitro como fuente de verdad | ✅ | El motor usa `turn.Board`; `warnIfIncoherent` reconstruye desde `moves` y solo avisa (`loop.go:127-139`) |
| Reintento del POST | ❌ | Ver 1.1 |
| Log por turno sin token ni headers | ✅ | `loop.go:99-100`: `game_id`, `column`, `depth`, `nodes`, `think_ms`, `elapsed_ms`, `state`. Token enmascarado en `config.go:98-121`; test `TestErrorsNeverContainToken`; en la simulación, el token no aparece en los logs |
| Respuesta lenta (25 s) | ✅ | Long poll de 25 s respondido sin error (timeout de 35 s) |
| Respuesta malformada | ✅ | Log `malformed arbiter response`, espera 1 s y sigue |
| 401 | ✅ | Log `HTTP 401 unauthorized`, espera 1 s y sigue (reintenta para siempre: correcto si el token se corrige en caliente, ruidoso en los logs) |
| Caída y vuelta del servidor | ✅ | `TestAuditLoopRecoversWhenServerComesBack`: 3 errores de conexión rechazada en 3 s; al volver el servidor en la misma dirección, juega el turno |
| POST cortado | ❌ | 4,5 s → partida perdida (1.1) |

## 5. Juego limpio

| Punto | Estado | Evidencia |
|---|---|---|
| Solo endpoints del protocolo | ✅ | El bot llama solo `GET /api/next` y `POST /api/move` (`client.go:53, 70`). `/api/me`, `/api/games/{id}` y `/mock/friendly` aparecen únicamente en `cmd/selfplay`, contra el mock. La web lee `GET /api/games/{id}` (público) solo para mostrar repeticiones (`web/src/app/replay/arbiter-record.service.ts:23`) |
| No usa datos de otras partidas o equipos para decidir | ✅ | `ChooseMove` recibe solo `board`, `you` y `legal_moves` del turno (`chooser.go:21`) |
| Sin trucos de tiempo | ✅ | `served_at` se guarda solo para logs (`parse.go:28`); el tiempo se mide desde la recepción (`loop.go:80`) |
| Un solo proceso por token | ⚠️ | Ver 1.3. No hay lock: el supervisor no impide dos instancias |
| Token solo desde el entorno, nunca en logs ni en el repo | ✅ | `config.go:31`; `.env` y `.env.*` en `.gitignore`. En `git log -p --all` no aparece ningún token real: solo `secret-team-token` y `selfplay-*` (tests) y el ejemplo `mi-token` del README |

## 6. Calidad y pruebas

| Punto | Estado | Evidencia |
|---|---|---|
| Comando único con cobertura | ✅ | `npm test` → `go test ./... -race -cover` + covercheck + tests del front |
| Cobertura de game y engine ≥ 70 % | ✅ | 100 % y 97 %, exigido por paquete (D12) |
| Verde desde un clon limpio | ✅ | `git clone` + `npm run test:bot` |
| Separación reglas / motor / cliente / API | ✅ | `internal/game` e `internal/engine` no importan `net/http` (verificado con `rg`) |
| Archivos chicos | ✅ | El archivo de producción más grande es `cmd/mock/server.go` (655 líneas); en el bot, ≤ 196 |
| Tests de comportamiento | ✅ | Tableros dibujados y resultados esperados; los fakes del loop simulan al árbitro, no verifican llamadas |
| Reglas: orientación, `Apply`, `FindWin` en bordes y esquinas, empate a 56, columna llena | ✅ | Tests existentes, más los de la revisión: las 107 ventanas × 4 casillas × 2 jugadores, diagonal que toca la fila 6 y la columna 7, esquinas, diagonal larga por el centro, fila 6 completa, columna 7 completa, siete en línea, empate con la ficha 56. **Ninguno falló contra el código** |

### Desviaciones del plan que faltan en `docs/TECNICO.md`

D1 a D13 ya documentan el resto. Faltan estas:

1. **`TT_ENABLED` sin efecto.** `config.go:64-69` lo lee y valida, pero ningún código lo usa: la fase 2 (paso 9: bitboard y TT) no se hizo.
2. **El timeout del POST (5 s) supera el plazo del árbitro (3 s).** Lo fija `AGENTS.md`, pero no tiene sentido frente al OpenAPI (ver 1.1).
3. **Paso 10: calibración con los `ms` reales del árbitro.** Pendiente para el jueves.
4. **El PDF de las bases no está en `docs/bases/`.** `AGENTS.md` y el plan dicen que está.

## 7. Errores de la IA en esta revisión (para `docs/bitacora-ia.md`)

Las horas son aproximadas: la revisión corrió entre las 22:30 y las 23:15.

| hora | quién | herramienta | qué se pidió | qué falló | cómo se detectó | decisión |
|---|---|---|---|---|---|---|
| 4 nov ≈23:00 | Matias | Claude Code | Tableros de prueba para `FindWin` | El tablero de la "diagonal larga por el centro" tenía un 2 en (1,2): el test fallaba por el dibujo, no por el código | `FindWin` devolvió una vertical válida de 2; revisión celda por celda | Se corrigió el dibujo, no el código |
| 4 nov ≈23:00 | Matias | Claude Code | Simulación del árbitro con `httptest` | El turno falso serializaba `legal_moves` como `[]game.Column` (`[]uint8`), que `encoding/json` convierte en base64; el bot lo rechazaba y el test se colgaba | Timeout del test; el código real ya usa `[]int` (`api/games.go:109`) | Se pasó a `[]int` en el test |
| 4 nov ≈23:00 | Matias | Claude Code | Medir el tiempo de punta a punta | El primer turno de prueba era táctico: la red de seguridad respondía en 0 ms y el test "pasaba" sin medir nada | El log mostró `0s` | Se usa una posición real de medio juego generada por el motor |
| 4 nov ≈23:00 | Matias | Claude Code | Medir la profundidad en medio juego | Las secuencias de jugadas inventadas terminaban en posiciones tácticas o con un cuatro ya hecho | Profundidad 1 y un error de apertura inválida | Las posiciones se generan jugando motor contra motor |

---

## Orden propuesto de los cambios (uno por uno, con tu ok)

1. **Reintento inmediato del POST + memoria de la decisión** (1.1). Bajo riesgo, evita una derrota segura ante un corte de red.
2. **`net.Listen` antes del loop** (1.3). 20 minutos.
3. **Poda de jugadas perdedoras en negamax** (1.2, opción A). Se demuestra con la suite táctica, la tabla de profundidad y el self-play contra la versión actual y contra `MaxDepth = 4`.
4. **Tabla de transposición detrás de `TT_ENABLED`** (1.2, opción B), solo si queda tiempo después de 3.
5. **Test de punta a punta ≤ 1 800 ms en la suite normal**, más las desviaciones en `TECNICO.md` y las líneas de la bitácora.

Lo que **no** se propone hoy: cambiar los pesos de la heurística (2.1) ni agregar paridad a la evaluación (1.2, opción C) sin un A/B de 20 partidas o más.

## Archivos de la revisión (sin commitear)

- `bot/internal/game/audit_rules_test.go`: tests de reglas que se suman a la suite normal (todos pasan).
- `bot/internal/engine/audit_test.go` (`-tags audit`): suite táctica, profundidad, self-play y análisis de derrotas. `go test -tags audit -run AuditTactics -v ./internal/engine`
- `bot/internal/arbiter/audit_test.go` (`-tags audit`): árbitro hostil, tiempo de punta a punta, caída y vuelta del servidor. `go test -tags audit -run Audit -v -timeout 5m ./internal/arbiter`

Los archivos con `-tags audit` no corren en `npm test`: el self-play tarda ~3 minutos y el árbitro hostil 30 s.

## Resultado contra el mock

Binario `cmd/bot` contra `cmd/mock` por HTTP (`POST /api/practice`, `first` 1 = abrimos nosotros, 2 = abre la casa), `THINK_MS=1500`, `POLL_WAIT=5`:

| Rival | Abriendo | Respondiendo | `ms` máximo medido por el mock |
|---|---|---|---|
| `casa-aleatorio` | 5 G de 5 (7 a 13 jugadas) | 5 G de 5 (10 a 14 jugadas) | 1 752 |
| `casa-basico` | 5 G de 5 (7 a 19 jugadas) | 5 G de 5 (8 a 24 jugadas) | 1 752 |

132 turnos jugados, 0 líneas `ERROR` o `WARN`, profundidad media 9,3 en las 108 jugadas que usaron la búsqueda, y el token no aparece en el log. Línea típica:

```
INFO turn game_id=g_p_01 column=3 depth=10 nodes=3909632 think_ms=1750 elapsed_ms=1750 state=playing
```

---

## Resultado de los fixes (4 nov 2026, 23:00–23:50)

Se trabajó en la rama `fix/revision-bot`, creada desde `main` (`94aceeb`). Durante la noche los cambios quedaron sin commitear, por indicación de Matias, porque el guard bloquea los commits sin un humano que confirme. Después, con Matias presente, se hizo un commit por caso y uno de documentación (tabla de abajo). No se hizo push ni merge, y no se tocó `main`.

Verificaciones después de los tres casos, con su salida:

- `gofmt -l .`: vacío.
- `go vet ./...` y `go vet -tags audit ./...`: limpios.
- `npm test` en verde con `-race`: `internal/game` 100,0 % (43/43), `internal/engine` 97,4 % (186/191), covercheck `ok`, y los 162 tests del front.

### 1.1 Reintento del `POST /api/move` — hecho (D14)

**Qué cambió.** `internal/arbiter/loop.go`.

- Si el `POST` falla por la red, se reenvía la misma columna cada 50 ms hasta 2 600 ms desde la recepción del turno.
- Cada intento tiene un `context.WithTimeout` con el tiempo que queda (mínimo 300 ms).
- `400` y `409` no se reintentan.
- El loop recuerda la última decisión (`game_id` + número de turno → columna). Si el turno se vuelve a entregar, juega esa columna sin pensar.
- La línea de log de cada turno suma `attempts`.

**Tests nuevos** (suite normal, en `move_retry_test.go`):

- `TestMoveIsRetriedAfterACutConnection`
- `TestMoveRetriesStopAtTheDeadlineAndTheLoopSurvives`
- `TestRedeliveredTurnReusesTheDecision`
- `TestReceptionToMoveUnder1800ms`: pasó de `-tags audit` a la suite normal, con tolerancia +150 ms bajo `-race` (`race_on_test.go` / `race_off_test.go`).

Los tres primeros fallaron antes del fix. El cuarto pasaba antes y pasa después. La posición de 16 fichas quedó fija en el test (`midgameMoves`), en lugar de generarse con el motor en cada corrida.

| Medición | Antes | Después |
|---|---|---|
| `TestAuditLoopSurvivesHostileArbiter`: primera entrega → jugada aceptada, con un `POST` cortado | 4,504 s (timeout) | **1,802 s** |
| `TestMoveIsRetriedAfterACutConnection`, con el motor falso de 1 750 ms | jugada nunca aceptada en 10 s | **1,81 s** |
| `TestReceptionToMoveUnder1800ms` | 1 751 ms | 1 755 ms |

Risk: un reintento podría duplicar la jugada si el árbitro no respondiera `409` a un `POST` repetido. El OpenAPI 1.0.1 dice que responde `409 not_your_turn` y que no penaliza. Si el árbitro real se comportara distinto, se vería en el log como `move rejected`.

### 1.3 Abrir el puerto antes del loop — hecho (D15)

**Qué cambió.** `cmd/bot/main.go`. `main` delega en `run(ctx, cfg, warm)`, que hace `net.Listen(":"+PORT)` antes del warm-up y del loop, y sirve con `srv.Serve(ln)`. Con el puerto ocupado termina con código 1 y `ERROR bot stopped err="listen on port … (is another bot already running?)"`.

**Tests nuevos:**

- `TestBusyPortFailsBeforeTalkingToTheArbiter`: el árbitro simulado recibió 2 276 requests antes del fix y 0 después.
- `TestRunServesHealthAndStopsOnCancel`.

La cobertura de `cmd/bot` pasó de 16,7 % a 61,1 %. A mano, con dos binarios en el mismo puerto, el segundo sale enseguida con código 1.

Risk: si `PORT` apunta a un puerto ocupado por otro programa, el bot ya no juega en el árbitro; antes jugaba igual, aunque sin API local. El supervisor lo relanza cada ~1 s y el error aparece en cada intento.

### 1.2 opción A, poda de jugadas perdedoras — hecho y conservado (D16)

**Qué cambió.** `internal/engine/search.go` y `ordering.go` (función `candidates`). En cada nodo interior:

1. Si se gana en una jugada, se devuelve la victoria sin explorar.
2. Si el rival tiene dos amenazas jugables, se devuelve la derrota. Si tiene una, solo se explora el bloqueo.
3. Las jugadas que le dejan al rival una casilla ganadora encima van al final.

La heurística y la raíz no cambiaron. Sin asignaciones: `BenchmarkSearchDepth8` da 0 B/op y 0 allocs/op.

**Tests nuevos.**

- En `prune_test.go`, con tableros dibujados: `TestNodeWithImmediateWinStopsAtOnce`, `TestNodeFacingTwoThreatsIsLost`, `TestNodeFacingOneThreatOnlyBlocks` y `TestMovesUnderARivalWinGoLast`. Los cuatro fallaron con una versión de `candidates` equivalente a la búsqueda anterior.
- En `audit_test.go` del motor:
  - `baselineChoose`, la copia literal de la búsqueda anterior;
  - `TestAuditVsBaseline`;
  - `TestAuditDepth`, que ahora genera las posiciones con baseline y mide los dos motores en cada una.

| Criterio (con `-tags audit`, 4 partidas en paralelo) | Antes | Después | ¿Cumple? |
|---|---|---|---|
| a. `TestAuditTactics` | 8/8 | 8/8 | ✅ |
| b. `TestAuditDepth`, mismas posiciones, a 8 / 12 / 16 / 24 fichas | 10 / 10 / 10 / 11 | 13 / 11 / 12 / 12 | ✅ |
| c. Nuevo vs `baselineChoose`, 20 partidas (10 aperturas × 2 colores) | — | 12 G / 0 E / 8 P | ✅ (pierde menos de lo que gana) |
| d. `TestAuditSelfPlayHouse` | 40/40 | 40/40 | ✅ |
| e. `TestAuditSelfPlayDepth4` | 15 G / 0 E / 5 P | 16 G / 0 E / 4 P | mejoró en 1 |

**Detalle de las mediciones.**

- **Victoria forzada.** Con 32 fichas, la victoria forzada a 19 plies aparece en 89 ms, contra 899 ms antes. Los nodos por segundo bajan de ~2,4 M a ~2,0 M, porque cada nodo hace más trabajo.
- **Color en el criterio c.** Como jugador 1, el motor nuevo ganó 9 de 10, contra 7 de 10 de baseline. Como jugador 2 ganó 3 de 10, contra 1 de 10.
- **Muestra chica.** 20 partidas no alcanzan para afirmar una mejora con significancia estadística.
- **Test de reporte.** `TestAuditSelfPlayDepth4` sigue terminando en FAIL porque exige 0 derrotas. Es un test de reporte y no forma parte de `npm test`.

Risk: un error en `candidates` haría que el motor descarte la única defensa o vea derrotas que no son. Lo cubren los 4 tests de la poda, la suite táctica 8/8 y `TestNeverIllegal` (1 000 posiciones). Para volver atrás alcanza con `git restore bot/internal/engine/search.go bot/internal/engine/ordering.go` y borrar `prune_test.go`. `baselineChoose` es la versión anterior exacta.

### Resultado contra el mock, después de los fixes

Binario `cmd/bot` de esta rama contra `cmd/mock`, con `THINK_MS=1500` y `POLL_WAIT=5`. Se jugaron 20 prácticas de a una (5 por rival y por color) para que ningún turno espere más de 30 s.

| Rival | Abriendo | Respondiendo | `ms` máximo medido por el mock |
|---|---|---|---|
| `casa-aleatorio` | 5 G de 5 (7 a 13 jugadas) | 5 G de 5 (8 a 18 jugadas) | 1 751 |
| `casa-basico` | 5 G de 5 (7 a 21 jugadas) | 5 G de 5 (8 a 18 jugadas) | 1 763 |

**Detalle.**

- Hubo 123 turnos jugados y 0 líneas `ERROR` o `WARN`.
- El token no aparece en el log.
- Todas las jugadas salieron con `attempts=1`.
- La profundidad media fue de 10,6 en las 100 jugadas que usaron la búsqueda; antes de los fixes era 9,3.
- El `ms` máximo subió de 1 752 a **1 763**: una sola jugada, todavía debajo de 1 800. Conviene mirarlo en la práctica real del jueves (D19).

### Lo que quedó sin hacer

- **Commits.** Hechos después, uno por caso, con los archivos de la tabla de abajo.
- **D19 en `TECNICO.md`** (calibración con los `ms` reales). El guard de archivos secretos bloqueó la escritura, porque el texto nombra el archivo de entorno del bot. Es un falso positivo, pero no se esquivó. El texto quedó listo para pegar después de D18: está en el scratchpad de la sesión de Claude Code (`D19-pendiente.md`) y también en el resumen de la conversación.
- **Caso 1.2, opción B (tabla de transposición), y opción C (paridad).** No se hicieron, como estaba previsto.

| Commit sugerido | Archivos |
|---|---|
| `fix(arbiter): reintenta el POST de la jugada y recuerda la última decisión` | `bot/internal/arbiter/loop.go`, `bot/internal/arbiter/move_retry_test.go`, `bot/internal/arbiter/race_on_test.go`, `bot/internal/arbiter/race_off_test.go`, `bot/internal/arbiter/audit_test.go` |
| `fix(bot): abre el puerto antes del warm-up y del loop del árbitro` | `bot/cmd/bot/main.go`, `bot/cmd/bot/main_test.go` |
| `feat(engine): poda jugadas perdedoras dentro del árbol` | `bot/internal/engine/search.go`, `bot/internal/engine/ordering.go`, `bot/internal/engine/prune_test.go`, `bot/internal/engine/audit_test.go` |
| `docs: agrega la revisión técnica, D14–D20 y la bitácora de la noche` | `bot/internal/game/audit_rules_test.go`, `docs/revision-bot.md`, `docs/TECNICO.md`, `docs/bitacora-ia.md` |

Comandos para verificar a la mañana, en la raíz del repo y en la rama `fix/revision-bot`: `git status --short && npm test && cd bot && go test -tags audit -count=1 -run 'AuditTactics|AuditDepth' -v ./internal/engine && go test -tags audit -count=1 -run AuditLoopSurvivesHostileArbiter -v -timeout 3m ./internal/arbiter && go test -run '^$' -bench . -benchmem ./internal/engine`
