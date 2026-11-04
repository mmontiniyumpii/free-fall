import { CellPosition } from './game-state';
import { lineDirection, sortWinningCells } from './winning-line';

describe('winning-line', () => {
  it('ordena las celdas a lo largo de la línea aunque lleguen desordenadas', () => {
    const cells: CellPosition[] = [[3, 5], [0, 2], [2, 4], [1, 3]];

    expect(sortWinningCells(cells)).toEqual([[0, 2], [1, 3], [2, 4], [3, 5]]);
  });

  it('en vertical ordena de abajo hacia arriba', () => {
    expect(sortWinningCells([[3, 6], [1, 6], [0, 6], [2, 6]])).toEqual([[0, 6], [1, 6], [2, 6], [3, 6]]);
  });

  it('reconoce horizontal, vertical y las dos diagonales', () => {
    expect(lineDirection([[2, 1], [2, 2], [2, 3], [2, 4]])).toBe('horizontal');
    expect(lineDirection([[0, 6], [1, 6], [2, 6], [3, 6]])).toBe('vertical');
    expect(lineDirection([[0, 2], [1, 3], [2, 4], [3, 5]])).toBe('diagonal');
    expect(lineDirection([[3, 0], [2, 1], [1, 2], [0, 3]])).toBe('diagonal');
  });
});
