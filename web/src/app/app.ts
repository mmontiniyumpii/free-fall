import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { Observable, finalize } from 'rxjs';

import { Board } from './board/board';
import { GameApi } from './game/game-api';
import { FirstPlayer, GameState } from './game/game-state';
import { Status } from './status/status';
import { Toolbar, ToolbarMode } from './toolbar/toolbar';

@Component({
  selector: 'app-root',
  imports: [Board, Status, Toolbar],
  templateUrl: './app.html',
  styleUrl: './app.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class App {
  private readonly api = inject(GameApi);

  /** Único estado de la partida: lo que devolvió la API, sin derivar reglas. `null` en el inicio. */
  protected readonly game = signal<GameState | null>(null);
  /** Pantalla de inicio o partida (incluye el instante en que se está creando). */
  protected readonly phase = signal<'setup' | 'game'>('setup');
  protected readonly first = signal<FirstPlayer>('human');
  /** Hay un pedido en curso: el tablero queda bloqueado hasta la respuesta. */
  protected readonly busy = signal(false);
  protected readonly focusedColumn = signal<number | null>(null);
  protected readonly rejectedColumn = signal<number | null>(null);

  /** Mientras hay un pedido, piensa el bot salvo al crear una partida que abre el humano. */
  protected readonly thinking = computed(
    () => this.busy() && (this.game() !== null || this.first() === 'bot'),
  );

  protected readonly toolbarMode = computed<ToolbarMode>(() => {
    const game = this.game();
    if (this.phase() === 'setup') return 'setup';
    return game && game.status !== 'playing' ? 'over' : 'playing';
  });

  protected readonly humanTurn = computed(() => {
    const game = this.game();
    return !this.busy() && game?.status === 'playing' && game.to_move === game.human;
  });

  /** Crea una partida con la configuración elegida; sirve para empezar y para reiniciar. */
  protected newGame(): void {
    if (this.busy()) return;
    this.phase.set('game');
    this.game.set(null);
    this.request(this.api.create(this.first()));
  }

  protected backToSetup(): void {
    if (this.busy()) return;
    this.phase.set('setup');
    this.game.set(null);
    this.resetHints();
  }

  protected play(column: number): void {
    const game = this.game();
    if (!game) return;
    this.request(this.api.move(game.id, column));
  }

  private request(call: Observable<GameState>): void {
    if (this.busy()) return;
    this.busy.set(true);
    this.resetHints();
    call
      .pipe(finalize(() => this.busy.set(false)))
      // Los errores (jugada ilegal, sin conexión) se muestran en el paso 4.
      .subscribe({ next: (state) => this.game.set(state), error: () => undefined });
  }

  private resetHints(): void {
    this.focusedColumn.set(null);
    this.rejectedColumn.set(null);
  }
}
