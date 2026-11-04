import { HttpErrorResponse } from '@angular/common/http';
import { DOCUMENT, Injectable, InjectionToken, inject } from '@angular/core';
import { Observable, delay, of, throwError } from 'rxjs';

import { GameApi } from './game-api';
import { FakeScenario, fakeState, parseScenario } from './fake-scenarios';
import { ApiError, Difficulty, FirstPlayer, GameState, ROWS } from './game-state';
import { landingRow } from './landing-row';

/** Escenario del fake; en el navegador se elige con `?scenario=won|draw|illegal_move`. */
export const FAKE_SCENARIO = new InjectionToken<FakeScenario>('FAKE_SCENARIO', {
  factory: () =>
    parseScenario(new URLSearchParams(inject(DOCUMENT).location.search).get('scenario')),
});

/** Simula la respuesta del bot sin superar el límite real de think_ms. */
const BOT_THINK_MS = 600;

/**
 * API falsa mientras el backend no existe. Parte de un estado fijo por
 * escenario y reproduce los errores del contrato leyendo `status` y
 * `legal_moves`. El bot no juega ni se detecta ganador: solo se agrega la ficha
 * del humano, con la misma `landingRow` que usa el tablero para dibujar.
 */
@Injectable()
export class FakeGameApiService extends GameApi {
  private readonly scenario = inject(FAKE_SCENARIO);
  private game: GameState | null = null;

  create(_first: FirstPlayer, difficulty?: Difficulty): Observable<GameState> {
    // Como el backend, devuelve la dificultad con la que se creó la partida.
    const state = fakeState(this.scenario);
    this.game = { ...state, difficulty: difficulty ?? state.difficulty };
    return of(structuredClone(this.game));
  }

  move(id: string, column: number): Observable<GameState> {
    const state = this.current();
    if (state.status !== 'playing') {
      return apiError(409, 'game_over', 'La partida ya terminó.');
    }
    const row = landingRow(state.board, column);
    if (this.scenario === 'illegal_move' || !state.legal_moves.includes(column) || row === null) {
      return apiError(400, 'illegal_move', `La columna ${column} está llena.`);
    }
    const board = state.board.map((cells) => [...cells]);
    board[row][column] = state.human;
    this.game = {
      ...state,
      id,
      board,
      history: [...state.history, column],
      legal_moves: row === ROWS - 1 ? state.legal_moves.filter((c) => c !== column) : state.legal_moves,
    };
    return of(structuredClone(this.game)).pipe(delay(BOT_THINK_MS));
  }

  get(id: string): Observable<GameState> {
    return of({ ...structuredClone(this.current()), id });
  }

  private current(): GameState {
    return this.game ?? fakeState(this.scenario);
  }
}

function apiError(status: number, error: ApiError['error'], message: string): Observable<never> {
  const body: ApiError = { ok: false, error, message };
  return throwError(() => new HttpErrorResponse({ status, error: body }));
}
