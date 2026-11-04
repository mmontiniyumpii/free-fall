import openapi from '../../../../docs/bases/arbitro-openapi.json';

import { ArbiterGame, describeResult, parseGameRef, replayFrame, teamName } from './arbiter-game';
import mockRecord from './testdata/game-record.mock.json';

/** Ejemplo de GET /api/games/{game_id} del OpenAPI, tal cual. */
const example = (
  openapi as unknown as {
    paths: Record<string, { get: { responses: Record<string, { content: Record<string, { example: unknown }> }> } }>;
  }
).paths['/api/games/{game_id}'].get.responses['200'].content['application/json'].example as ArbiterGame;

/** Partida real del self-play contra el mock (42 jugadas, gana el jugador 2 en diagonal). */
const selfPlay = mockRecord as unknown as ArbiterGame;

describe('parseGameRef', () => {
  it('acepta el código o un link que lo contenga', () => {
    expect(parseGameRef('g_7f3a')).toBe('g_7f3a');
    expect(parseGameRef('  g_f_01 ')).toBe('g_f_01');
    expect(parseGameRef('https://arbitro.yompi.com/api/games/g_7f3a')).toBe('g_7f3a');
    expect(parseGameRef('https://arbitro.yompi.com/games/g_7f3a?x=1')).toBe('g_7f3a');
  });

  it('rechaza lo que no parece un código', () => {
    expect(parseGameRef('')).toBeNull();
    expect(parseGameRef('hola mundo')).toBeNull();
  });
});

describe('replayFrame', () => {
  it('al final reproduce exactamente el tablero del registro (fila 0 abajo)', () => {
    for (const game of [example, selfPlay]) {
      expect(replayFrame(game, game.moves.length).board).toEqual(game.board);
    }
  });

  it('el paso 0 es el tablero vacío y le toca a quien abre', () => {
    const frame = replayFrame(selfPlay, 0);

    expect(frame.board.flat().every((cell) => cell === 0)).toBe(true);
    expect(frame.to_move).toBe(1);
    expect(frame.status).toBe('playing');
  });

  it('cada ficha es de quien dice `by` en el registro, apilada desde abajo', () => {
    // Ejemplo del OpenAPI: el jugador 1 juega la columna 3 y el 2 la columna 2.
    const frame = replayFrame(example, 2);

    expect(frame.board[0][3]).toBe(1);
    expect(frame.board[0][2]).toBe(2);
    expect(frame.history).toEqual([3, 2]);
  });

  it('el resultado y la línea ganadora solo aparecen en el último paso', () => {
    const last = replayFrame(selfPlay, selfPlay.moves.length);
    const before = replayFrame(selfPlay, selfPlay.moves.length - 1);

    expect(last.status).toBe('won');
    expect(last.winner).toBe(2);
    expect(last.winning_cells).toEqual(selfPlay.result?.winning_cells);
    expect(before.status).toBe('playing');
    expect(before.winning_cells).toBeNull();
  });

  it('pasos fuera de rango se acotan a la partida', () => {
    expect(replayFrame(selfPlay, 999).history.length).toBe(selfPlay.moves.length);
    expect(replayFrame(selfPlay, -3).history).toEqual([]);
  });

  it('una derrota por tiempo no dibuja línea ganadora', () => {
    const game: ArbiterGame = { ...selfPlay, result: { winner: 1, reason: 'timeout' } };

    expect(replayFrame(game, game.moves.length).winning_cells).toBeNull();
  });
});

describe('describeResult', () => {
  const withResult = (result: ArbiterGame['result']): ArbiterGame => ({ ...selfPlay, result });

  it('explica cada forma de terminar con los nombres de los equipos', () => {
    expect(describeResult(selfPlay)).toBe('Ganó Equipo 2 con cuatro en línea.');
    expect(describeResult(withResult({ winner: null, reason: 'draw' }))).toBe('Empate: se llenó el tablero.');
    expect(describeResult(withResult({ winner: 1, reason: 'timeout' }))).toBe(
      'Ganó Equipo 1: Equipo 2 se pasó de tiempo.',
    );
    expect(describeResult(withResult({ winner: 2, reason: 'illegal_move' }))).toBe(
      'Ganó Equipo 2: Equipo 1 hizo una jugada ilegal.',
    );
    expect(describeResult(withResult({ winner: 2, reason: 'absent' }))).toBe(
      'Ganó Equipo 2: Equipo 1 no recogió su turno a tiempo.',
    );
    expect(describeResult(withResult(null))).toBe('Partida en curso: se ve hasta la última jugada.');
  });

  it('sin nombre de equipo usa el número de jugador', () => {
    expect(teamName({ ...selfPlay, players: {} }, 2)).toBe('Jugador 2');
  });
});
