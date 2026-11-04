import { HttpErrorResponse } from '@angular/common/http';
import { DOCUMENT, Injectable, InjectionToken, inject } from '@angular/core';
import { Observable, delay, of, throwError } from 'rxjs';

import { GameApi } from './game-api';
import { FakeScenario, fakeState, parseScenario } from './fake-scenarios';
import { ApiError, Difficulty, FirstPlayer, GameState } from './game-state';

/** Escenario del fake; en el navegador se elige con `?scenario=won|draw|illegal_move`. */
export const FAKE_SCENARIO = new InjectionToken<FakeScenario>('FAKE_SCENARIO', {
  factory: () =>
    parseScenario(new URLSearchParams(inject(DOCUMENT).location.search).get('scenario')),
});

/** Simula la respuesta del bot sin superar el límite real de think_ms. */
const BOT_THINK_MS = 600;

/**
 * API falsa mientras el backend no existe. No juega: devuelve estados fijos y
 * reproduce los errores del contrato leyendo `status` y `legal_moves`.
 */
@Injectable()
export class FakeGameApiService extends GameApi {
  private readonly scenario = inject(FAKE_SCENARIO);

  create(_first: FirstPlayer, difficulty?: Difficulty): Observable<GameState> {
    // Como el backend, devuelve la dificultad con la que se creó la partida.
    const state = fakeState(this.scenario);
    return of({ ...state, difficulty: difficulty ?? state.difficulty });
  }

  move(id: string, column: number): Observable<GameState> {
    const state = fakeState(this.scenario);
    if (state.status !== 'playing') {
      return apiError(409, 'game_over', 'La partida ya terminó.');
    }
    if (this.scenario === 'illegal_move' || !state.legal_moves.includes(column)) {
      return apiError(400, 'illegal_move', `La columna ${column} está llena.`);
    }
    return of({ ...state, id }).pipe(delay(BOT_THINK_MS));
  }

  get(id: string): Observable<GameState> {
    return of({ ...fakeState(this.scenario), id });
  }
}

function apiError(status: number, error: ApiError['error'], message: string): Observable<never> {
  const body: ApiError = { ok: false, error, message };
  return throwError(() => new HttpErrorResponse({ status, error: body }));
}
