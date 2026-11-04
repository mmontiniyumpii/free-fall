import { GameState } from './game-state';

export interface MoveEntry {
  /** Número de jugada, desde 1. */
  number: number;
  column: number;
  owner: 'human' | 'bot';
}

/**
 * Jugadas de `history` con quién hizo cada una, leído del tablero que manda el
 * backend: la ficha de la jugada i en la columna c es la (k+1)-ésima de esa
 * columna desde abajo, con k = veces que se jugó c antes. No supone el orden
 * de turnos; solo que las fichas se apilan desde abajo, como en landingRow.
 */
export function moveHistory(state: GameState): MoveEntry[] {
  const playedInColumn = new Map<number, number>();
  return state.history.flatMap((column, index) => {
    const row = playedInColumn.get(column) ?? 0;
    playedInColumn.set(column, row + 1);
    const cell = state.board[row]?.[column] ?? 0;
    // Un history inconsistente con el tablero no se dibuja en vez de inventar un dueño.
    if (cell === 0) return [];
    return [{ number: index + 1, column, owner: cell === state.human ? 'human' : 'bot' }];
  });
}
