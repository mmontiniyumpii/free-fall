import { DOCUMENT, InjectionToken, inject } from '@angular/core';
import { Observable } from 'rxjs';

import { environment } from '../../environments/environment';
import { Difficulty, FirstPlayer, GameState } from './game-state';

/** Puerto de entrada a la API local; lo implementan el servicio HTTP y el falso. */
export abstract class GameApi {
  /** Sin `difficulty`, el backend usa `hard`. */
  abstract create(first: FirstPlayer, difficulty?: Difficulty): Observable<GameState>;
  abstract move(id: string, column: number): Observable<GameState>;
  abstract get(id: string): Observable<GameState>;
}

/**
 * La API vive en el mismo host que sirve la página: así la app funciona
 * abierta desde otra máquina de la red local o desde un teléfono.
 */
export function apiUrlFor(location: Pick<Location, 'protocol' | 'hostname'>, port: number): string {
  return `${location.protocol}//${location.hostname}:${port}`;
}

export const API_URL = new InjectionToken<string>('API_URL', {
  factory: () => apiUrlFor(inject(DOCUMENT).location, environment.apiPort),
});
