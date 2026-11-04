import { fakeState } from './fake-scenarios';
import { GameState } from './game-state';
import { moveHistory } from './move-history';

describe('moveHistory', () => {
  it('lee del tablero quién hizo cada jugada', () => {
    // Fixture: history 3,4,3,4,3,3; columna 3 con 1,1,1,2 desde abajo y columna 4 con 2,2.
    expect(moveHistory(fakeState('fixture'))).toEqual([
      { number: 1, column: 3, owner: 'human' },
      { number: 2, column: 4, owner: 'bot' },
      { number: 3, column: 3, owner: 'human' },
      { number: 4, column: 4, owner: 'bot' },
      { number: 5, column: 3, owner: 'human' },
      { number: 6, column: 3, owner: 'bot' },
    ]);
  });

  it('cuando abre el bot (humano = 2), la primera jugada es del bot', () => {
    expect(moveHistory(fakeState('playing', 'bot'))).toEqual([
      { number: 1, column: 3, owner: 'bot' },
    ]);
  });

  it('no supone turnos alternados: el dueño sale siempre del tablero', () => {
    const state: GameState = {
      ...fakeState('playing', 'human'),
      history: [0, 1],
      board: fakeState('playing', 'human').board.map((row, r) =>
        r === 0 ? [1, 1, 0, 0, 0, 0, 0, 0] : row,
      ),
    };

    expect(moveHistory(state).map((entry) => entry.owner)).toEqual(['human', 'human']);
  });

  it('una jugada sin ficha en el tablero no se inventa', () => {
    const state: GameState = { ...fakeState('playing', 'human'), history: [5] };

    expect(moveHistory(state)).toEqual([]);
  });
});
