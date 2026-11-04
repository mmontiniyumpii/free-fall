import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';

import { DIFFICULTIES } from '../game/difficulty';
import { Difficulty, FirstPlayer } from '../game/game-state';

export type ToolbarMode = 'setup' | 'playing' | 'offline' | 'over';

interface StarterOption {
  value: FirstPlayer;
  label: string;
  detail: string;
}

const STARTERS: readonly StarterOption[] = [
  { value: 'human', label: 'Vos', detail: 'Tu color' },
  { value: 'bot', label: 'El bot', detail: 'Fichas azules' },
];

/** Acciones del panel: elegir quién abre, empezar, reiniciar y volver al inicio. */
@Component({
  selector: 'app-toolbar',
  templateUrl: './toolbar.html',
  styleUrl: './toolbar.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Toolbar {
  readonly mode = input.required<ToolbarMode>();
  readonly first = input.required<FirstPlayer>();
  readonly difficulty = input.required<Difficulty>();
  readonly busy = input(false);
  /** Muestra la ayuda de las teclas 1 a 8 (turno del humano en escritorio). */
  readonly showKeys = input(false);

  readonly firstChange = output<FirstPlayer>();
  readonly difficultyChange = output<Difficulty>();
  /** Crea una partida con la configuración elegida (inicio y reinicio). */
  readonly newGame = output<void>();
  readonly backToSetup = output<void>();
  /** Abre la reproducción de una partida del torneo. */
  readonly openReplay = output<void>();
  /** Reintenta ya el pedido que falló por falta de conexión. */
  readonly retry = output<void>();

  protected readonly starters = STARTERS;
  protected readonly difficulties = DIFFICULTIES;

  protected readonly difficultyDetail = computed(
    () => DIFFICULTIES.find((option) => option.value === this.difficulty())?.detail ?? '',
  );
}
