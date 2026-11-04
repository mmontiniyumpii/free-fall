import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { GameState } from '../game/game-state';
import { landingRow } from '../game/landing-row';

type Side = 'human' | 'bot';

/** Mensaje que reemplaza al estado normal, por ejemplo si la partida se perdió. */
export interface StatusNotice {
  title: string;
  detail: string;
}

interface StatusView {
  eyebrow: string;
  title: string;
  detail: string;
  /** Título y detalle en ámbar (sin conexión). */
  alert: boolean;
  /** El detalle es un aviso (columna llena). */
  warning: boolean;
  /** De quién es el turno en el indicador; null lo oculta. */
  turn: Side | null;
}

/**
 * Bloque de estado del diseño: eyebrow, título, detalle e indicador Vos/Bot.
 * Solo traduce el GameState a texto; no decide nada del juego.
 */
@Component({
  selector: 'app-status',
  templateUrl: './status.html',
  styleUrl: './status.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Status {
  /** `null` en la pantalla de inicio. */
  readonly state = input.required<GameState | null>();
  /** Hay un pedido en curso y el que juega es el bot. */
  readonly thinking = input(false);
  readonly focusedColumn = input<number | null>(null);
  readonly rejectedColumn = input<number | null>(null);
  /** Segundos hasta el próximo reintento; null si hay conexión. */
  readonly retryIn = input<number | null>(null);
  readonly notice = input<StatusNotice | null>(null);

  protected readonly view = computed<StatusView>(() => {
    const state = this.state();
    const moves = state?.history.length ?? 0;
    const notice = this.notice();
    const retryIn = this.retryIn();

    if (notice) {
      return { eyebrow: 'Partida interrumpida', ...notice, alert: false, warning: false, turn: null };
    }

    if (retryIn !== null) {
      return {
        eyebrow: `Jugada ${moves + 1}`,
        title: 'Sin conexión',
        detail: `No se pudo conectar con el bot. Reintentamos en ${retryIn} s.`,
        alert: true,
        warning: false,
        turn: null,
      };
    }

    if (this.thinking()) {
      return {
        eyebrow: `Jugada ${moves + 1}`,
        title: 'El bot está pensando',
        detail: 'Responde en menos de 2 segundos.',
        alert: false,
        warning: false,
        turn: 'bot',
      };
    }

    if (!state) {
      return {
        eyebrow: 'Nueva partida',
        title: 'Cuatro en línea',
        detail: 'Conectá cuatro fichas antes que el bot: en horizontal, vertical o diagonal.',
        alert: false,
        warning: false,
        turn: null,
      };
    }

    if (state.status === 'draw') {
      return {
        eyebrow: `Fin de la partida · ${moves} jugadas`,
        title: 'Empate',
        detail: 'Se llenaron las 56 casillas sin ganador.',
        alert: false,
        warning: false,
        turn: null,
      };
    }

    if (state.status === 'won') {
      const humanWon = state.winner === state.human;
      return {
        eyebrow: `Fin de la partida · ${moves} jugadas`,
        title: humanWon ? 'Ganaste' : 'Ganó el bot',
        detail: humanWon ? 'Conectaste cuatro.' : 'El bot conectó cuatro.',
        alert: false,
        warning: false,
        turn: null,
      };
    }

    if (state.to_move !== state.human) {
      return {
        eyebrow: `Jugada ${moves + 1}`,
        title: 'Turno del bot',
        detail: 'Esperando la jugada del bot.',
        alert: false,
        warning: false,
        turn: 'bot',
      };
    }

    return {
      eyebrow: `Jugada ${moves + 1}`,
      title: 'Tu turno',
      alert: false,
      ...this.humanTurnDetail(state),
      turn: 'human',
    };
  });

  private humanTurnDetail(state: GameState): Pick<StatusView, 'detail' | 'warning'> {
    const rejected = this.rejectedColumn();
    if (rejected !== null) {
      return { detail: `La columna ${rejected + 1} está llena. Elegí otra.`, warning: true };
    }
    const focused = this.focusedColumn();
    const row = focused === null ? null : landingRow(state.board, focused);
    if (focused !== null && row !== null) {
      return { detail: `Columna ${focused + 1} · cae en la fila ${row + 1}.`, warning: false };
    }
    return { detail: 'Elegí una columna.', warning: false };
  }
}
