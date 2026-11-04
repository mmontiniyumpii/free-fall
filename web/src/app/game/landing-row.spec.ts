import { fakeState } from './fake-scenarios';
import { Cell } from './game-state';
import { landingRow } from './landing-row';

describe('landingRow', () => {
  it('en una columna vacía cae en la fila 0 (abajo)', () => {
    expect(landingRow(fakeState('playing').board, 0)).toBe(0);
  });

  it('cae sobre la última ficha de la columna', () => {
    // Fixture: columna 3 con fichas en las filas 0 a 3.
    expect(landingRow(fakeState('playing').board, 3)).toBe(4);
  });

  it('una columna llena no tiene fila', () => {
    const board: Cell[][] = Array.from({ length: 7 }, (_, row) =>
      Array<Cell>(8).fill(row % 2 === 0 ? 1 : 2),
    );
    expect(landingRow(board, 5)).toBeNull();
  });
});
