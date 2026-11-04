// Estados fijos para FakeGameApiService. Son datos, no reglas: el front no los
// valida. `playing` es el fixture del contrato, sin copiarlo.
import fixture from '../../../../docs/fixtures/game-state.example.json';

import { GameState } from './game-state';

export type FakeScenario = 'playing' | 'won' | 'draw' | 'illegal_move';

export const FAKE_SCENARIOS: readonly FakeScenario[] = ['playing', 'won', 'draw', 'illegal_move'];

// El JSON se importa con tipos anchos (number, string); la forma se verifica en
// fake-scenarios.spec.ts.
const playing = fixture as GameState;

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

const STATES: Record<FakeScenario, GameState> = {
  playing,
  won,
  draw,
  // Mismo tablero que `playing`; la diferencia está en FakeGameApiService.move.
  illegal_move: playing,
};

/** Copia profunda: quien la reciba puede mutarla sin tocar los datos fijos. */
export function fakeState(scenario: FakeScenario): GameState {
  return structuredClone(STATES[scenario]);
}

export function parseScenario(value: string | null): FakeScenario {
  return FAKE_SCENARIOS.find((scenario) => scenario === value) ?? 'playing';
}
