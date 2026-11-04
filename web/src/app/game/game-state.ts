// Tipos del contrato de la API local (docs/api-local.md). El front no calcula
// reglas: legalidad, ganador y línea de cuatro llegan siempre en el GameState.

export const ROWS = 7;
export const COLUMNS = 8;

/** 1 abrió la partida, 2 respondió. */
export type Player = 1 | 2;

/** 0 vacío, 1 o 2 según el jugador. */
export type Cell = 0 | Player;

export type GameStatus = 'playing' | 'won' | 'draw';

export type FirstPlayer = 'human' | 'bot';

/** Limita la profundidad de búsqueda del bot en la API local; no afecta al torneo. */
export type Difficulty = 'easy' | 'medium' | 'hard';

/** [fila, columna], fila 0 abajo. */
export type CellPosition = [row: number, column: number];

export interface LastBotMove {
  column: number;
  depth: number;
  nodes: number;
  elapsed_ms: number;
}

export interface GameState {
  id: string;
  /** board[fila][columna], 7 × 8, fila 0 es la de abajo. */
  board: Cell[][];
  to_move: Player | null;
  legal_moves: number[];
  status: GameStatus;
  winner: Player | null;
  winning_cells: CellPosition[] | null;
  history: number[];
  human: Player;
  bot: Player;
  difficulty: Difficulty;
  last_bot: LastBotMove | null;
}

export type ApiErrorCode =
  | 'illegal_move'
  | 'validation'
  | 'unknown_game'
  | 'game_over'
  | 'not_your_turn';

export interface ApiError {
  ok: false;
  error: ApiErrorCode;
  message: string;
}
