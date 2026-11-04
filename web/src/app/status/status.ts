import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { GameState } from '../game/game-state';
import { landingRow } from '../game/landing-row';

type Side = 'human' | 'bot';

interface StatusView {
  eyebrow: string;
  title: string;
  detail: string;
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

  protected readonly view = computed<StatusView>(() => {
    const state = this.state();
    const moves = state?.history.length ?? 0;

    if (this.thinking()) {
      return {
        eyebrow: `Jugada ${moves + 1}`,
        title: 'El bot está pensando',
        detail: 'Responde en menos de 2 segundos.',
        warning: false,
        turn: 'bot',
      };
    }

    if (!state) {
      return {
        eyebrow: 'Nueva partida',
        title: 'Cuatro en línea',
        detail: 'Conectá cuatro fichas antes que el bot: en horizontal, vertical o diagonal.',
        warning: false,
        turn: null,
      };
    }

    if (state.status === 'draw') {
      return {
        eyebrow: `Fin de la partida · ${moves} jugadas`,
        title: 'Empate',
        detail: 'Se llenaron las 56 casillas sin ganador.',
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
        warning: false,
        turn: null,
      };
    }

    if (state.to_move !== state.human) {
      return {
        eyebrow: `Jugada ${moves + 1}`,
        title: 'Turno del bot',
        detail: 'Esperando la jugada del bot.',
        warning: false,
        turn: 'bot',
      };
    }

    return {
      eyebrow: `Jugada ${moves + 1}`,
      title: 'Tu turno',
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
