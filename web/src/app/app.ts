import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  InjectionToken,
  computed,
  inject,
  signal,
} from '@angular/core';
import { Observable } from 'rxjs';

import { Board } from './board/board';
import { Confetti } from './confetti/confetti';
import { TOUCH_INPUT } from './device';
import { USING_FAKE_API } from './game/fake-game-api.service';
import { History } from './history/history';
import { Replay } from './replay/replay';
import { Rules } from './rules/rules';
import { classifyError } from './game/api-error';
import { GameApi } from './game/game-api';
import { DEFAULT_DIFFICULTY } from './game/difficulty';
import { ApiError, Difficulty, FirstPlayer, GameState } from './game/game-state';
import { Status, StatusNotice } from './status/status';
import { Toolbar, ToolbarMode } from './toolbar/toolbar';

/** Segundos de espera antes de reintentar sin conexión (diseño 07). */
export const RETRY_SECONDS = new InjectionToken<number>('RETRY_SECONDS', { factory: () => 5 });

/** Cuánto se muestra el aviso de columna llena que devolvió el bot. */
const REJECTED_MS = 1800;

/** Pedido que quedó sin respuesta y hay que repetir al recuperar la conexión. */
type Unanswered =
  | { kind: 'create' }
  | { kind: 'move'; gameId: string; column: number; historyLength: number }
  | { kind: 'sync'; gameId: string };

@Component({
  selector: 'app-root',
  imports: [Board, Confetti, History, Replay, Rules, Status, Toolbar],
  templateUrl: './app.html',
  styleUrl: './app.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class App {
  private readonly api = inject(GameApi);
  private readonly retrySeconds = inject(RETRY_SECONDS);
  protected readonly touch = inject(TOUCH_INPUT);
  /** Con ?scenario= juega un bot de prueba: se avisa siempre para no confundirlo con el real. */
  protected readonly usingFake = inject(USING_FAKE_API);

  /** Único estado de la partida: lo que devolvió la API, sin derivar reglas. `null` en el inicio. */
  protected readonly game = signal<GameState | null>(null);
  /** Inicio, partida contra el bot (incluye el instante en que se crea) o reproducción. */
  protected readonly phase = signal<'setup' | 'game' | 'replay'>('setup');
  /** Tablero de la partida del torneo que se está reproduciendo. */
  protected readonly replayFrame = signal<GameState | null>(null);

  /** Lo que dibuja el tablero: la partida propia o el paso de la reproducción. */
  protected readonly boardState = computed(() =>
    this.phase() === 'replay' ? this.replayFrame() : this.game(),
  );
  protected readonly first = signal<FirstPlayer>('human');
  protected readonly difficulty = signal<Difficulty>(DEFAULT_DIFFICULTY);
  /** Hay un pedido en curso: el tablero queda bloqueado hasta la respuesta. */
  protected readonly busy = signal(false);
  /** Jugada del humano enviada y sin respuesta; el tablero la dibuja cayendo. */
  protected readonly pendingColumn = signal<number | null>(null);
  /** Cuenta regresiva del reintento; null mientras hay conexión. */
  protected readonly retryIn = signal<number | null>(null);
  protected readonly notice = signal<StatusNotice | null>(null);
  protected readonly focusedColumn = signal<number | null>(null);
  protected readonly rejectedColumn = signal<number | null>(null);
  /** Modal de reglas abierto: el tablero no responde (ni a las teclas 1 a 8). */
  protected readonly rulesOpen = signal(false);

  private unanswered: Unanswered | null = null;
  private countdown: ReturnType<typeof setInterval> | undefined;
  private rejectedTimer: ReturnType<typeof setTimeout> | undefined;

  protected readonly offline = computed(() => this.retryIn() !== null);

  /** Mientras hay un pedido, piensa el bot salvo al crear una partida que abre el humano. */
  protected readonly thinking = computed(
    () => this.busy() && !this.offline() && (this.game() !== null || this.first() === 'bot'),
  );

  protected readonly boardLocked = computed(
    () =>
      this.busy() ||
      this.offline() ||
      this.notice() !== null ||
      this.rulesOpen() ||
      this.phase() === 'replay',
  );

  protected readonly toolbarMode = computed<ToolbarMode>(() => {
    const game = this.game();
    if (this.phase() === 'setup') return 'setup';
    if (this.offline()) return 'offline';
    if (this.notice() || (game && game.status !== 'playing')) return 'over';
    return 'playing';
  });

  /** Ganó el humano (según status, winner y human): lluvia de confeti sobre el tablero. */
  protected readonly celebrating = computed(() => {
    const game = this.game();
    return this.phase() === 'game' && game?.status === 'won' && game.winner === game.human;
  });

  protected readonly humanTurn = computed(() => {
    const game = this.game();
    return !this.boardLocked() && game?.status === 'playing' && game.to_move === game.human;
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => {
      clearInterval(this.countdown);
      clearTimeout(this.rejectedTimer);
    });
  }

  /** Crea una partida con la configuración elegida; sirve para empezar y para reiniciar. */
  protected newGame(): void {
    if (this.busy()) return;
    this.clearInterruption();
    this.phase.set('game');
    this.game.set(null);
    this.unanswered = { kind: 'create' };
    this.send(this.api.create(this.first(), this.difficulty()));
  }

  protected openReplay(): void {
    if (this.busy()) return;
    this.clearInterruption();
    this.game.set(null);
    this.phase.set('replay');
  }

  protected closeReplay(): void {
    this.replayFrame.set(null);
    this.phase.set('setup');
  }

  protected backToSetup(): void {
    if (this.busy()) return;
    this.clearInterruption();
    this.phase.set('setup');
    this.game.set(null);
  }

  protected play(column: number): void {
    const game = this.game();
    if (!game || this.boardLocked()) return;
    this.pendingColumn.set(column);
    this.unanswered = {
      kind: 'move',
      gameId: game.id,
      column,
      historyLength: game.history.length,
    };
    this.send(this.api.move(game.id, column));
  }

  /**
   * Repite el pedido sin respuesta. Una jugada nunca se reenvía a ciegas: el bot
   * pudo haberla aplicado y perdido la respuesta, así que primero se consulta la
   * partida y solo se reenvía si `history` no creció.
   */
  protected retry(): void {
    if (this.busy()) return;
    this.stopCountdown();
    const pending = this.unanswered;
    if (!pending) return;
    if (pending.kind === 'create') {
      this.send(this.api.create(this.first(), this.difficulty()));
      return;
    }
    this.busy.set(true);
    this.api.get(pending.gameId).subscribe({
      next: (state) => {
        const moveLost = pending.kind === 'move' && state.history.length === pending.historyLength;
        if (moveLost) {
          this.send(this.api.move(pending.gameId, pending.column));
        } else {
          this.accept(state);
        }
      },
      error: (error: unknown) => this.fail(error),
    });
  }

  private send(call: Observable<GameState>): void {
    this.busy.set(true);
    this.focusedColumn.set(null);
    call.subscribe({
      next: (state) => this.accept(state),
      error: (error: unknown) => this.fail(error),
    });
  }

  private accept(state: GameState): void {
    this.busy.set(false);
    this.unanswered = null;
    this.pendingColumn.set(null);
    this.game.set(state);
  }

  private fail(error: unknown): void {
    this.busy.set(false);
    const failure = classifyError(error);
    if (failure === 'offline') {
      this.startCountdown();
      return;
    }
    const column = this.pendingColumn();
    this.pendingColumn.set(null);
    this.unanswered = null;
    switch (failure) {
      case 'illegal_move':
        if (column !== null) this.flagRejected(column);
        this.resync();
        break;
      case 'conflict':
        this.resync();
        break;
      case 'lost':
        this.notice.set({
          title: 'La partida ya no existe',
          detail: 'El bot se reinició y la partida se perdió. Empezá una nueva.',
        });
        break;
      default:
        this.notice.set({
          title: 'Algo salió mal',
          detail: errorMessage(error) ?? 'El bot respondió con un error inesperado.',
        });
    }
  }

  /** El front quedó desfasado del bot: se trae el estado real de la partida. */
  private resync(): void {
    const game = this.game();
    if (!game) return;
    this.unanswered = { kind: 'sync', gameId: game.id };
    this.send(this.api.get(game.id));
  }

  private startCountdown(): void {
    this.stopCountdown();
    this.retryIn.set(this.retrySeconds);
    this.countdown = setInterval(() => {
      const next = (this.retryIn() ?? 0) - 1;
      if (next <= 0) {
        this.retry();
      } else {
        this.retryIn.set(next);
      }
    }, 1000);
  }

  private stopCountdown(): void {
    clearInterval(this.countdown);
    this.retryIn.set(null);
  }

  private flagRejected(column: number): void {
    this.rejectedColumn.set(column);
    clearTimeout(this.rejectedTimer);
    this.rejectedTimer = setTimeout(() => this.rejectedColumn.set(null), REJECTED_MS);
  }

  private clearInterruption(): void {
    this.stopCountdown();
    this.notice.set(null);
    this.pendingColumn.set(null);
    this.unanswered = null;
    this.focusedColumn.set(null);
    this.rejectedColumn.set(null);
  }
}

function errorMessage(error: unknown): string | null {
  const body = (error as { error?: Partial<ApiError> } | null)?.error;
  return typeof body?.message === 'string' ? body.message : null;
}
