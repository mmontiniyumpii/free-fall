import { HttpErrorResponse } from '@angular/common/http';
import { DOCUMENT, Injectable, InjectionToken, inject } from '@angular/core';
import { Observable, delay, of, throwError } from 'rxjs';

import { GameApi } from './game-api';
import { FakeScenario, fakeState, parseScenario } from './fake-scenarios';
import { ApiError, Difficulty, FirstPlayer, GameState, Player } from './game-state';
import { landingRow } from './landing-row';

/**
 * Escenario del fake; en el navegador se elige con
 * `?scenario=fixture|won|bot_won|draw|illegal_move|offline|lost`.
 */
export const FAKE_SCENARIO = new InjectionToken<FakeScenario>('FAKE_SCENARIO', {
  factory: () =>
    parseScenario(new URLSearchParams(inject(DOCUMENT).location.search).get('scenario')),
});

/** Cuánto tarda el bot falso en responder: lo mismo que el motor real (think_ms). Los tests lo bajan a 0. */
export const FAKE_THINK_MS = new InjectionToken<number>('FAKE_THINK_MS', { factory: () => 1500 });

/**
 * La app usa la API real del bot salvo que la URL pida un escenario del fake
 * (`?scenario=...`), para mostrar estados que no se pueden provocar a mano.
 */
export function fakeRequested(search: string): boolean {
  return new URLSearchParams(search).has('scenario');
}

/** El bot falso no decide: juega la primera columna disponible en este orden fijo. */
const FAKE_BOT_ORDER = [3, 4, 2, 5, 1, 6, 0, 7];

/**
 * API falsa mientras el backend no existe. Parte de un estado fijo por
 * escenario y reproduce los errores del contrato leyendo `status` y
 * `legal_moves`. No detecta ganador ni empate: agrega la ficha del humano y una
 * respuesta fija del bot, con la misma `landingRow` que usa el tablero.
 *
 * - `offline`: la primera jugada se aplica pero la respuesta se pierde (status
 *   0), para probar que el reintento no juega dos veces.
 * - `lost`: las jugadas responden 404 unknown_game (el bot se reinició).
 */
@Injectable()
export class FakeGameApiService extends GameApi {
  private readonly scenario = inject(FAKE_SCENARIO);
  private readonly thinkMs = inject(FAKE_THINK_MS);
  private game: GameState | null = null;
  private dropNextResponse = this.scenario === 'offline';

  create(first: FirstPlayer, difficulty?: Difficulty): Observable<GameState> {
    // Como el backend, devuelve la dificultad con la que se creó la partida.
    const state = fakeState(this.scenario, first);
    this.game = { ...state, difficulty: difficulty ?? state.difficulty };
    // Si abre el bot, la respuesta ya trae su jugada: tarda lo que tarda en pensar.
    const created = of(structuredClone(this.game));
    return first === 'bot' ? created.pipe(delay(this.thinkMs)) : created;
  }

  move(id: string, column: number): Observable<GameState> {
    const state = this.current();
    if (this.scenario === 'lost') {
      return apiError(404, 'unknown_game', `No existe la partida ${id}.`);
    }
    if (state.status !== 'playing') {
      return apiError(409, 'game_over', 'La partida ya terminó.');
    }
    if (this.scenario === 'illegal_move' || !state.legal_moves.includes(column)) {
      return apiError(400, 'illegal_move', `La columna ${column} está llena.`);
    }
    const afterHuman = drop(state, column, state.human);
    const botColumn = FAKE_BOT_ORDER.find((c) => afterHuman.legal_moves.includes(c));
    const afterBot =
      botColumn === undefined
        ? afterHuman
        : {
            ...drop(afterHuman, botColumn, state.bot),
            last_bot: { column: botColumn, depth: 4, nodes: 1200, elapsed_ms: this.thinkMs },
          };
    this.game = { ...afterBot, id };
    if (this.dropNextResponse) {
      this.dropNextResponse = false;
      return throwError(() => new HttpErrorResponse({ status: 0, statusText: 'Unknown Error' })).pipe(
        delay(this.thinkMs),
      );
    }
    return of(structuredClone(this.game)).pipe(delay(this.thinkMs));
  }

  get(id: string): Observable<GameState> {
    return of({ ...structuredClone(this.current()), id });
  }

  private current(): GameState {
    return this.game ?? fakeState(this.scenario);
  }
}

/** Agrega una ficha con landingRow y quita la columna de legal_moves si se llenó. */
function drop(state: GameState, column: number, player: Player): GameState {
  const row = landingRow(state.board, column);
  if (row === null) return state;
  const board = state.board.map((cells) => [...cells]);
  board[row][column] = player;
  const full = landingRow(board, column) === null;
  return {
    ...state,
    board,
    history: [...state.history, column],
    legal_moves: full ? state.legal_moves.filter((c) => c !== column) : state.legal_moves,
  };
}

function apiError(status: number, error: ApiError['error'], message: string): Observable<never> {
  const body: ApiError = { ok: false, error, message };
  return throwError(() => new HttpErrorResponse({ status, error: body }));
}
