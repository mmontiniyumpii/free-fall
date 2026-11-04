import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable, timeout } from 'rxjs';

import { API_URL, GameApi } from './game-api';
import { Difficulty, FirstPlayer, GameState } from './game-state';

/**
 * Sin respuesta en este tiempo se considera que el bot no está. Cubre de sobra
 * think_ms (1 500 ms) + ~100 ms que puede tardar POST /moves.
 */
export const REQUEST_TIMEOUT_MS = 10_000;

/** Cliente HTTP de la API local del bot (docs/api-local.md). */
@Injectable()
export class GameApiService extends GameApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = inject(API_URL);

  create(first: FirstPlayer, difficulty?: Difficulty): Observable<GameState> {
    const body = difficulty ? { first, difficulty } : { first };
    return this.http.post<GameState>(`${this.baseUrl}/local/games`, body).pipe(timeout(REQUEST_TIMEOUT_MS));
  }

  move(id: string, column: number): Observable<GameState> {
    return this.http
      .post<GameState>(`${this.gameUrl(id)}/moves`, { column })
      .pipe(timeout(REQUEST_TIMEOUT_MS));
  }

  get(id: string): Observable<GameState> {
    return this.http.get<GameState>(this.gameUrl(id)).pipe(timeout(REQUEST_TIMEOUT_MS));
  }

  private gameUrl(id: string): string {
    return `${this.baseUrl}/local/games/${encodeURIComponent(id)}`;
  }
}
