import { HttpErrorResponse } from '@angular/common/http';
import { TimeoutError } from 'rxjs';

import { ApiError } from './game-state';

/**
 * Qué hace la app ante cada falla de la API local (docs/api-local.md):
 * - offline: no hubo respuesta útil (red, 5xx, timeout); se reintenta.
 * - illegal_move: la columna no se podía jugar; se avisa y se resincroniza.
 * - conflict: game_over o not_your_turn; el front quedó desfasado y se resincroniza.
 * - lost: la partida no existe (el bot se reinició); hay que empezar otra.
 * - other: cualquier otro error del contrato (por ejemplo validation).
 */
export type ApiFailure = 'offline' | 'illegal_move' | 'conflict' | 'lost' | 'other';

export function classifyError(error: unknown): ApiFailure {
  if (error instanceof TimeoutError) return 'offline';
  if (!(error instanceof HttpErrorResponse)) return 'other';
  if (error.status === 0 || error.status >= 500) return 'offline';
  switch (apiErrorBody(error)?.error) {
    case 'illegal_move':
      return 'illegal_move';
    case 'game_over':
    case 'not_your_turn':
      return 'conflict';
    case 'unknown_game':
      return 'lost';
    default:
      return 'other';
  }
}

function apiErrorBody(error: HttpErrorResponse): ApiError | null {
  const body: unknown = error.error;
  if (typeof body === 'object' && body !== null && 'error' in body) return body as ApiError;
  return null;
}
