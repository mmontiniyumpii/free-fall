import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

import { COLUMNS, Cell, GameState, ROWS } from '../game/game-state';

type Owner = 'human' | 'bot' | null;

interface CellView {
  row: number;
  owner: Owner;
}

interface ColumnView {
  index: number;
  label: number;
  cells: CellView[];
}

// El contrato tiene la fila 0 abajo; el DOM dibuja de arriba hacia abajo. La
// inversión vive solo acá, al renderizar.
const ROWS_TOP_TO_BOTTOM = Array.from({ length: ROWS }, (_, i) => ROWS - 1 - i);

@Component({
  selector: 'app-board',
  templateUrl: './board.html',
  styleUrl: './board.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
})
export class Board {
  readonly state = input.required<GameState>();

  protected readonly columns = computed<ColumnView[]>(() => {
    const state = this.state();
    return Array.from({ length: COLUMNS }, (_, column) => ({
      index: column,
      label: column + 1,
      cells: ROWS_TOP_TO_BOTTOM.map((row) => ({
        row,
        owner: ownerOf(state, state.board[row][column]),
      })),
    }));
  });
}

function ownerOf(state: GameState, cell: Cell): Owner {
  if (cell === 0) return null;
  return cell === state.human ? 'human' : 'bot';
}
