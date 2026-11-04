import { Cell } from './game-state';

/**
 * Fila vacía más baja de una columna (fila 0 abajo), o null si está llena.
 * Solo para dibujar: dónde caería la ficha y el texto "cae en la fila N".
 * La legalidad de una jugada sale siempre de `legal_moves`, nunca de acá.
 */
export function landingRow(board: Cell[][], column: number): number | null {
  for (let row = 0; row < board.length; row++) {
    if (board[row][column] === 0) return row;
  }
  return null;
}
