import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { GameState } from '../game/game-state';
import { moveHistory } from '../game/move-history';

/** Historial de jugadas del panel, de la más nueva a la más vieja (diseño). */
@Component({
  selector: 'app-history',
  templateUrl: './history.html',
  styleUrl: './history.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class History {
  readonly state = input.required<GameState>();

  protected readonly entries = computed(() => moveHistory(this.state()).reverse());

  protected readonly count = computed(() => {
    const total = this.state().history.length;
    return total === 1 ? '1 jugada' : `${total} jugadas`;
  });
}
