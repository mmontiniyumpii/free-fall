import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { finalize } from 'rxjs';

import { Board } from './board/board';
import { GameApi } from './game/game-api';
import { GameState } from './game/game-state';

@Component({
  selector: 'app-root',
  imports: [Board],
  templateUrl: './app.html',
  styleUrl: './app.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class App {
  private readonly api = inject(GameApi);

  /** Único estado de la partida: lo que devolvió la API, sin derivar reglas. */
  protected readonly game = signal<GameState | null>(null);
  /** Hay un pedido en curso: el tablero queda bloqueado hasta la respuesta. */
  protected readonly busy = signal(false);

  constructor() {
    // Paso 1: arranca con una partida para mostrar el tablero. La pantalla de
    // inicio (quién abre) llega en el paso 3.
    this.api.create('human').subscribe((state) => this.game.set(state));
  }

  protected play(column: number): void {
    const game = this.game();
    if (!game || this.busy()) return;
    this.busy.set(true);
    this.api
      .move(game.id, column)
      .pipe(finalize(() => this.busy.set(false)))
      // Los errores (jugada ilegal, sin conexión) se muestran en el paso 4.
      .subscribe({ next: (state) => this.game.set(state), error: () => undefined });
  }
}
