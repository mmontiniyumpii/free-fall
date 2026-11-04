import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  effect,
  inject,
  output,
  signal,
} from '@angular/core';

import { classifyError } from '../game/api-error';
import { GameState, Player } from '../game/game-state';
import {
  ArbiterGame,
  KIND_LABELS,
  describeResult,
  parseGameRef,
  replayFrame,
  teamName,
} from './arbiter-game';
import { ArbiterRecordService } from './arbiter-record.service';

/** Pausa entre jugadas al reproducir. */
const PLAY_INTERVAL_MS = 700;

interface MoveRow {
  step: number;
  turn: number;
  by: Player;
  team: string;
  column: number;
  ms: number | null;
}

/**
 * Reproduce una partida del torneo a partir de su registro público en el
 * árbitro. Solo lectura: el tablero que se ve lo arma replayFrame.
 */
@Component({
  selector: 'app-replay',
  templateUrl: './replay.html',
  styleUrl: './replay.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '(document:keydown)': 'onKeydown($event)',
  },
})
export class Replay {
  private readonly records = inject(ArbiterRecordService);

  /** Tablero a dibujar en el paso actual; null mientras no hay partida cargada. */
  readonly frameChange = output<GameState | null>();
  readonly exit = output<void>();

  protected readonly query = signal('');
  protected readonly loading = signal(false);
  protected readonly error = signal<string | null>(null);
  protected readonly game = signal<ArbiterGame | null>(null);
  protected readonly step = signal(0);
  protected readonly playing = signal(false);
  private timer: ReturnType<typeof setInterval> | undefined;

  protected readonly total = computed(() => this.game()?.moves.length ?? 0);
  protected readonly atEnd = computed(() => this.step() === this.total());
  protected readonly kind = computed(() => {
    const game = this.game();
    return game ? KIND_LABELS[game.kind] : '';
  });
  protected readonly teams = computed(() => {
    const game = this.game();
    return game ? [teamName(game, 1), teamName(game, 2)] : [];
  });
  protected readonly result = computed(() => {
    const game = this.game();
    return game ? describeResult(game) : '';
  });
  protected readonly stepTitle = computed(() => {
    const step = this.step();
    return step === 0 ? 'Tablero vacío' : `Jugada ${step} de ${this.total()}`;
  });
  protected readonly rows = computed<MoveRow[]>(() => {
    const game = this.game();
    if (!game) return [];
    return game.moves.map((move, index) => ({
      step: index + 1,
      turn: move.turn,
      by: move.by,
      team: teamName(game, move.by),
      column: move.column,
      ms: move.ms ?? null,
    }));
  });

  constructor() {
    effect(() => {
      const game = this.game();
      this.frameChange.emit(game ? replayFrame(game, this.step()) : null);
    });
    inject(DestroyRef).onDestroy(() => clearInterval(this.timer));
  }

  protected load(): void {
    const id = parseGameRef(this.query());
    if (!id) {
      this.error.set('Escribí el código de la partida (por ejemplo g_7f3a) o pegá su link.');
      return;
    }
    this.pause();
    this.loading.set(true);
    this.error.set(null);
    this.records.get(id).subscribe({
      next: (game) => {
        this.loading.set(false);
        this.game.set(game);
        this.step.set(0);
      },
      error: (error: unknown) => {
        this.loading.set(false);
        this.error.set(loadError(error));
      },
    });
  }

  protected goTo(step: number): void {
    this.step.set(Math.max(0, Math.min(step, this.total())));
    if (this.atEnd()) this.pause();
  }

  protected togglePlay(): void {
    if (this.playing()) {
      this.pause();
      return;
    }
    if (this.atEnd()) this.step.set(0);
    this.playing.set(true);
    this.timer = setInterval(() => this.goTo(this.step() + 1), PLAY_INTERVAL_MS);
  }

  protected backToMenu(): void {
    this.pause();
    this.exit.emit();
  }

  /** Flechas ← → para avanzar o retroceder de a una jugada. */
  protected onKeydown(event: KeyboardEvent): void {
    if (!this.game() || event.target instanceof HTMLInputElement) return;
    if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return;
    // Sin esto, el navegador además desplaza el panel con la flecha.
    event.preventDefault();
    this.goTo(this.step() + (event.key === 'ArrowRight' ? 1 : -1));
  }

  protected onQuery(event: Event): void {
    this.query.set((event.target as HTMLInputElement).value);
  }

  private pause(): void {
    clearInterval(this.timer);
    this.playing.set(false);
  }
}

function loadError(error: unknown): string {
  switch (classifyError(error)) {
    case 'offline':
      return 'No se pudo conectar con el árbitro. Probá de nuevo en un rato.';
    case 'lost':
      return 'No existe una partida con ese código.';
    default:
      return (error as { status?: number })?.status === 404
        ? 'No existe una partida con ese código.'
        : 'El árbitro respondió con un error inesperado.';
  }
}
