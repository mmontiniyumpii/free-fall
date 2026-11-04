import { CellPosition } from './game-state';

export type LineDirection = 'horizontal' | 'vertical' | 'diagonal';

/**
 * Celdas de `winning_cells` ordenadas a lo largo de la línea (de izquierda a
 * derecha; en vertical, de abajo hacia arriba). El contrato no fija el orden y
 * el diseño anima los anillos en secuencia. Solo para dibujar: la línea la
 * decide siempre el backend.
 */
export function sortWinningCells(cells: readonly CellPosition[]): CellPosition[] {
  return [...cells].sort(([rowA, columnA], [rowB, columnB]) => columnA - columnB || rowA - rowB);
}

/** Dirección de una línea ya ordenada, para el texto del final. */
export function lineDirection(cells: readonly CellPosition[]): LineDirection {
  const [firstRow, firstColumn] = cells[0];
  const [lastRow, lastColumn] = cells[cells.length - 1];
  if (firstRow === lastRow) return 'horizontal';
  if (firstColumn === lastColumn) return 'vertical';
  return 'diagonal';
}
