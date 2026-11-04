import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { difficultyLabel } from '../game/difficulty';
import { GameState } from '../game/game-state';
import { landingRow } from '../game/landing-row';
import { lineDirection, sortWinningCells } from '../game/winning-line';

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
  /** Ganó el humano: el título entra con un "pop". */
  celebrate?: boolean;
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
  /** Toque en dos pasos: los textos de ayuda hablan de tocar. */
  readonly touch = input(false);

  /** Datos de la última jugada del bot (last_bot), como información. */
  protected readonly botInfo = computed<string | null>(() => {
    const last = this.state()?.last_bot;
    if (!last || this.thinking() || this.retryIn() !== null || this.notice()) return null;
    return [
      `columna ${last.column + 1}`,
      `profundidad ${last.depth}`,
      `${groupDigits(last.nodes)} nodos`,
      `${groupDigits(last.elapsed_ms)} ms`,
    ].join(' · ');
  });

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
        eyebrow: playEyebrow(state, moves),
        title: 'Sin conexión',
        detail: `No se pudo conectar con el bot. Reintentamos en ${retryIn} s.`,
        alert: true,
        warning: false,
        turn: null,
      };
    }

    if (this.thinking()) {
      return {
        eyebrow: playEyebrow(state, moves),
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
        celebrate: humanWon,
        detail: `${humanWon ? 'Conectaste' : 'Conectó'} cuatro${describeLine(state)}.`,
        alert: false,
        warning: false,
        turn: null,
      };
    }

    if (state.to_move !== state.human) {
      return {
        eyebrow: playEyebrow(state, moves),
        title: 'Turno del bot',
        detail: 'Esperando la jugada del bot.',
        alert: false,
        warning: false,
        turn: 'bot',
      };
    }

    return {
      eyebrow: playEyebrow(state, moves),
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
    if (this.touch()) {
      return focused === null
        ? { detail: 'Tocá una columna para ver dónde cae.', warning: false }
        : { detail: `Tocá de nuevo la columna ${focused + 1} para soltar.`, warning: false };
    }
    const row = focused === null ? null : landingRow(state.board, focused);
    if (focused !== null && row !== null) {
      return { detail: `Columna ${focused + 1} · cae en la fila ${row + 1}.`, warning: false };
    }
    return { detail: 'Elegí una columna.', warning: false };
  }
}

/** " en diagonal, de la columna 3 a la 6", leído de winning_cells (columnas desde 1). */
function describeLine(state: GameState): string {
  if (state.winning_cells?.length !== 4) return '';
  const line = sortWinningCells(state.winning_cells);
  const direction = lineDirection(line);
  const firstColumn = line[0][1] + 1;
  const lastColumn = line[line.length - 1][1] + 1;
  const where =
    direction === 'vertical'
      ? `en la columna ${firstColumn}`
      : `de la columna ${firstColumn} a la ${lastColumn}`;
  return ` en ${direction}, ${where}`;
}

/** 412380 → "412 380", con espacio fino como en el plan. */
function groupDigits(value: number): string {
  return String(value).replace(/\B(?=(\d{3})+(?!\d))/g, '\u202f');
}

/** "Jugada 5 · Fácil": la dificultad sale del GameState que devolvió el bot. */
function playEyebrow(state: GameState | null, moves: number): string {
  const play = `Jugada ${moves + 1}`;
  return state ? `${play} · ${difficultyLabel(state.difficulty)}` : play;
}
