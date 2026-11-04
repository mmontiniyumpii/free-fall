import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';

import { API_URL, GameApi } from './game-api';
import { Difficulty, FirstPlayer, GameState } from './game-state';

/** Cliente HTTP de la API local del bot (docs/api-local.md). */
@Injectable()
export class GameApiService extends GameApi {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = inject(API_URL);

  create(first: FirstPlayer, difficulty?: Difficulty): Observable<GameState> {
    const body = difficulty ? { first, difficulty } : { first };
    return this.http.post<GameState>(`${this.baseUrl}/local/games`, body);
  }

  move(id: string, column: number): Observable<GameState> {
    return this.http.post<GameState>(`${this.gameUrl(id)}/moves`, { column });
  }

  get(id: string): Observable<GameState> {
    return this.http.get<GameState>(this.gameUrl(id));
  }

  private gameUrl(id: string): string {
    return `${this.baseUrl}/local/games/${encodeURIComponent(id)}`;
  }
}
