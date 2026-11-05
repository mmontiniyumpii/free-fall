// Registro público de una partida del árbitro: GET /api/games/{game_id}
// (docs/bases/arbitro-openapi.json, schema Game). Solo se lee: el resultado y la
// línea ganadora vienen del árbitro.
import { Cell, CellPosition, COLUMNS, GameState, Player, ROWS } from '../game/game-state';
import { landingRow } from '../game/landing-row';

/** Schema PlayerRef (OpenAPI 1.0.1). */
export interface ArbiterTeam {
  team_id: string;
  name: string;
  /** Solo presente (en true) cuando el jugador es un bot de la casa. */
  house?: true;
}

export interface ArbiterMove {
  turn: number;
  by: Player;
  column: number;
  /** Milisegundos que tardó el bot desde served_at. */
  ms?: number;
}

/** admin: el árbitro dio la partida por perdida. */
export type ArbiterResultReason = 'connect4' | 'draw' | 'illegal_move' | 'timeout' | 'absent' | 'admin';

export interface ArbiterGame {
  game_id: string;
  match_id?: string | null;
  kind: 'practice' | 'friendly' | 'league' | 'third_place' | 'final';
  /** Serie de playoffs; null fuera de playoffs. */
  series?: 'third' | 'final' | null;
  /** scheduled: partida de liga que todavía no empezó. */
  status: 'scheduled' | 'playing' | 'finished';
  players: Partial<Record<'1' | '2', ArbiterTeam>>;
  moves: ArbiterMove[];
  /** Jugador al que le toca; null si la partida no está en curso. */
  current?: Player | null;
  board: Cell[][];
  result?: {
    winner: Player | null;
    reason: ArbiterResultReason;
    winning_cells?: CellPosition[];
    /** Quien perdió por jugada ilegal, tiempo, ausencia o decisión del árbitro. */
    forfeited_by?: Player;
  } | null;
  started_at?: string | null;
  finished_at?: string | null;
}

export const KIND_LABELS: Record<ArbiterGame['kind'], string> = {
  practice: 'Práctica',
  friendly: 'Amistoso',
  league: 'Liga',
  third_place: 'Tercer puesto',
  final: 'Final',
};

/**
 * Acepta el código de la partida o un link que lo contenga
 * (".../games/g_7f3a"). Devuelve null si no parece un código.
 */
export function parseGameRef(text: string): string | null {
  const value = text.trim();
  const fromUrl = /\/games\/([\w-]+)/.exec(value);
  if (fromUrl) return fromUrl[1];
  return /^[\w-]+$/.test(value) ? value : null;
}

export function teamName(game: ArbiterGame, player: Player): string {
  return game.players[String(player) as '1' | '2']?.name ?? `Jugador ${player}`;
}

/**
 * Estado del tablero después de las primeras `step` jugadas, en la forma que
 * dibuja el tablero. Cada ficha es de quien dice `by` en el registro; solo se
 * usa la gravedad (landingRow) para apilar. El jugador 1 se dibuja como ficha
 * llena y el 2 como hueca. Al final se muestran resultado y línea del árbitro.
 */
export function replayFrame(game: ArbiterGame, step: number): GameState {
  const moves = game.moves.slice(0, Math.max(0, Math.min(step, game.moves.length)));
  const board: Cell[][] = Array.from({ length: ROWS }, () => Array<Cell>(COLUMNS).fill(0));
  for (const move of moves) {
    const row = landingRow(board, move.column);
    if (row !== null) board[row][move.column] = move.by;
  }
  const atEnd = moves.length === game.moves.length && game.result != null;
  const cells = game.result?.winning_cells;
  const won = atEnd && game.result?.winner != null;
  return {
    id: game.game_id,
    board,
    to_move: atEnd ? null : (game.moves[moves.length]?.by ?? null),
    legal_moves: [],
    status: atEnd ? (won ? 'won' : 'draw') : 'playing',
    winner: won ? (game.result?.winner ?? null) : null,
    winning_cells: won && cells?.length === 4 ? cells : null,
    history: moves.map((move) => move.column),
    human: 1,
    bot: 2,
    difficulty: 'hard',
    last_bot: null,
  };
}

/** Cómo terminó, en una frase para el panel. */
export function describeResult(game: ArbiterGame): string {
  const result = game.result;
  if (game.status === 'scheduled') return 'Partida programada: todavía no empezó.';
  if (!result) return 'Partida en curso: se ve hasta la última jugada.';
  if (result.winner === null) return 'Empate: se llenó el tablero.';
  const winner = teamName(game, result.winner);
  const loser = teamName(game, result.winner === 1 ? 2 : 1);
  switch (result.reason) {
    case 'draw':
      return 'Empate: se llenó el tablero.';
    case 'connect4':
      return `Ganó ${winner} con cuatro en línea.`;
    case 'illegal_move':
      return `Ganó ${winner}: ${loser} hizo una jugada ilegal.`;
    case 'timeout':
      return `Ganó ${winner}: ${loser} se pasó de tiempo.`;
    case 'absent':
      return `Ganó ${winner}: ${loser} no recogió su turno a tiempo.`;
    case 'admin':
      return `Ganó ${winner}: el árbitro le dio la partida por perdida a ${loser}.`;
  }
}
