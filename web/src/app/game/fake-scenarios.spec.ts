import { FAKE_SCENARIOS, fakeState, parseScenario } from './fake-scenarios';

// El fixture JSON entra con tipos anchos: este test verifica que todos los
// estados del fake respetan la forma del GameState de docs/api-local.md.
describe('escenarios del fake', () => {
  for (const scenario of FAKE_SCENARIOS) {
    it(`${scenario}: cumple la forma del GameState del contrato`, () => {
      const state = fakeState(scenario);

      expect(state.board.length).toBe(7);
      for (const row of state.board) {
        expect(row.length).toBe(8);
        for (const cell of row) expect([0, 1, 2]).toContain(cell);
      }
      expect(['playing', 'won', 'draw']).toContain(state.status);
      expect(['easy', 'medium', 'hard']).toContain(state.difficulty);
      expect([state.human, state.bot].sort()).toEqual([1, 2]);
      expect(state.history.length).toBe(state.board.flat().filter((cell) => cell !== 0).length);
      if (state.status === 'won') {
        expect(state.winning_cells).toHaveLength(4);
        expect(state.to_move).toBeNull();
      } else {
        expect(state.winning_cells).toBeNull();
      }
    });
  }

  it('cada pedido devuelve una copia: mutarla no cambia el escenario', () => {
    const first = fakeState('fixture');
    first.board[0][0] = 2;

    expect(fakeState('fixture').board[0][0]).toBe(0);
  });

  it('un escenario desconocido o ausente cae en playing', () => {
    expect(parseScenario('draw')).toBe('draw');
    expect(parseScenario('cualquiera')).toBe('playing');
    expect(parseScenario(null)).toBe('playing');
  });
});
