import { Difficulty } from './game-state';

export interface DifficultyOption {
  value: Difficulty;
  label: string;
  /** Fiel a lo que hace el motor (docs/TECNICO.md, D1): solo cambia la profundidad máxima. */
  detail: string;
}

export const DIFFICULTIES: readonly DifficultyOption[] = [
  { value: 'easy', label: 'Fácil', detail: 'Mira 2 jugadas adelante: gana y bloquea lo inmediato.' },
  { value: 'medium', label: 'Normal', detail: 'Mira 6 jugadas adelante: se le puede ganar planificando.' },
  { value: 'hard', label: 'Difícil', detail: 'Mira tan lejos como le da el tiempo: aprovecha cualquier error.' },
];

/** Por defecto, el mismo nivel que juega el torneo (y el default del backend). */
export const DEFAULT_DIFFICULTY: Difficulty = 'hard';

export function difficultyLabel(value: Difficulty): string {
  return DIFFICULTIES.find((option) => option.value === value)?.label ?? value;
}
