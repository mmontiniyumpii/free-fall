import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';

import { FirstPlayer } from '../game/game-state';

export type ToolbarMode = 'setup' | 'playing' | 'over';

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
  readonly busy = input(false);
  /** Muestra la ayuda de las teclas 1 a 8 (turno del humano en escritorio). */
  readonly showKeys = input(false);

  readonly firstChange = output<FirstPlayer>();
  /** Crea una partida con la configuración elegida (inicio y reinicio). */
  readonly newGame = output<void>();
  readonly backToSetup = output<void>();

  protected readonly starters = STARTERS;
}
