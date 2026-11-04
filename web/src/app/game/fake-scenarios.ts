// Estados fijos para FakeGameApiService. Son datos, no reglas: el front no los
// valida. `fixture` es el ejemplo del contrato, importado sin copiarlo.
import fixture from '../../../../docs/fixtures/game-state.example.json';

import { Cell, FirstPlayer, GameState } from './game-state';

export type FakeScenario =
  | 'playing'
  | 'fixture'
  | 'won'
  | 'draw'
  | 'illegal_move'
  | 'offline'
  | 'lost';

export const FAKE_SCENARIOS: readonly FakeScenario[] = [
  'playing',
  'fixture',
  'won',
  'draw',
  'illegal_move',
  'offline',
  'lost',
];

/** Escenarios que arrancan con una partida nueva; el resto parte de un estado fijo. */
type NewGameScenario = 'playing' | 'offline' | 'lost';

// El JSON se importa con tipos anchos (number, string); la forma se verifica en
// fake-scenarios.spec.ts.
const midGame = fixture as GameState;

function emptyBoard(): Cell[][] {
  return Array.from({ length: 7 }, () => Array<Cell>(8).fill(0));
}

/**
 * Partida nueva como la describe el contrato para POST /local/games. Si abre el
 * bot, su primera jugada es fija (columna 3): el fake no decide jugadas.
 */
function newGame(first: FirstPlayer): GameState {
  const base: GameState = {
    id: 'l_fake',
    board: emptyBoard(),
    to_move: 1,
    legal_moves: [0, 1, 2, 3, 4, 5, 6, 7],
    status: 'playing',
    winner: null,
    winning_cells: null,
    history: [],
    human: 1,
    bot: 2,
    difficulty: 'hard',
    last_bot: null,
  };
  if (first === 'human') return base;
  const board = emptyBoard();
  board[0][3] = 1;
  return {
    ...base,
    board,
    to_move: 2,
    history: [3],
    human: 2,
    bot: 1,
    last_bot: { column: 3, depth: 10, nodes: 523114, elapsed_ms: 1350 },
  };
}

// Ejemplo "partida ganada por el humano" de docs/api-local.md, con el tablero
// reconstruido desde su history: diagonal (0,2) (1,3) (2,4) (3,5) del jugador 1.
const won: GameState = {
  id: 'l_0002',
  board: [
    [0, 0, 1, 2, 2, 1, 1, 0],
    [0, 0, 0, 1, 2, 2, 0, 0],
    [0, 0, 0, 0, 1, 2, 0, 0],
    [0, 0, 0, 0, 0, 1, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0],
    [0, 0, 0, 0, 0, 0, 0, 0],
  ],
  to_move: null,
  legal_moves: [],
  status: 'won',
  winner: 1,
  winning_cells: [
    [0, 2],
    [1, 3],
    [2, 4],
    [3, 5],
  ],
  history: [2, 3, 3, 4, 5, 4, 4, 5, 6, 5, 5],
  human: 1,
  bot: 2,
  difficulty: 'medium',
  last_bot: { column: 5, depth: 6, nodes: 21004, elapsed_ms: 38 },
};

// Tablero lleno sin cuatro en línea. El history reproduce el tablero alternando
// 1 y 2 (verificado fuera de la app al armar el dato).
const draw: GameState = {
  id: 'l_0003',
  board: [
    [1, 1, 2, 2, 1, 1, 2, 2],
    [1, 1, 2, 2, 1, 1, 2, 2],
    [1, 1, 2, 2, 1, 1, 2, 2],
    [2, 2, 1, 1, 2, 2, 1, 1],
    [1, 1, 2, 2, 1, 1, 2, 2],
    [1, 1, 2, 2, 1, 1, 2, 2],
    [1, 1, 2, 2, 1, 1, 2, 2],
  ],
  to_move: null,
  legal_moves: [],
  status: 'draw',
  winner: null,
  winning_cells: null,
  history: [
    4, 3, 4, 3, 4, 3, 3, 3, 5, 3, 5, 3, 5, 4, 4, 2, 4, 2, 4, 2, 2, 2, 1, 2, 1, 2, 1, 5, 5, 1, 5, 6,
    5, 6, 1, 6, 1, 7, 1, 7, 6, 6, 0, 6, 0, 6, 0, 0, 0, 7, 7, 7, 0, 7, 0, 7,
  ],
  human: 1,
  bot: 2,
  difficulty: 'hard',
  last_bot: { column: 7, depth: 1, nodes: 1, elapsed_ms: 0 },
};

const STATES: Record<Exclude<FakeScenario, NewGameScenario>, GameState> = {
  fixture: midGame,
  won,
  draw,
  // Mismo tablero que `fixture`; la diferencia está en FakeGameApiService.move.
  illegal_move: midGame,
};

/**
 * Estado inicial de un escenario. `playing`, `offline` y `lost` respetan quién
 * abre; el resto es fijo. Copia profunda: quien la reciba puede mutarla sin
 * tocar los datos.
 */
export function fakeState(scenario: FakeScenario, first: FirstPlayer = 'human'): GameState {
  if (scenario === 'playing' || scenario === 'offline' || scenario === 'lost') {
    return newGame(first);
  }
  return structuredClone(STATES[scenario]);
}

export function parseScenario(value: string | null): FakeScenario {
  return FAKE_SCENARIOS.find((scenario) => scenario === value) ?? 'playing';
}
