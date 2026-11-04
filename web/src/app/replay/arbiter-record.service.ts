import { HttpClient } from '@angular/common/http';
import { Injectable, InjectionToken, inject } from '@angular/core';
import { Observable, timeout } from 'rxjs';

import { ArbiterGame } from './arbiter-game';

/**
 * Prefijo del proxy del servidor de desarrollo (web/proxy.conf.mjs) que reenvía
 * al árbitro definido en ARBITRO_URL.
 */
export const ARBITER_BASE = new InjectionToken<string>('ARBITER_BASE', { factory: () => '/arbitro' });

const RECORD_TIMEOUT_MS = 10_000;

/** Lee el registro público de una partida del árbitro (no necesita token). */
@Injectable({ providedIn: 'root' })
export class ArbiterRecordService {
  private readonly http = inject(HttpClient);
  private readonly base = inject(ARBITER_BASE);

  get(gameId: string): Observable<ArbiterGame> {
    return this.http
      .get<ArbiterGame>(`${this.base}/api/games/${encodeURIComponent(gameId)}`)
      .pipe(timeout(RECORD_TIMEOUT_MS));
  }
}
