import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';

import { COLUMNS, Cell, GameState, ROWS } from '../game/game-state';
import { landingRow } from '../game/landing-row';

type Owner = 'human' | 'bot' | null;

interface CellView {
  row: number;
  owner: Owner;
  ghost: boolean;
}

interface ColumnView {
  index: number;
  label: number;
  legal: boolean;
  focused: boolean;
  invalid: boolean;
  cells: CellView[];
}

// El contrato tiene la fila 0 abajo; el DOM dibuja de arriba hacia abajo. La
// inversión vive solo acá, al renderizar.
const ROWS_TOP_TO_BOTTOM = Array.from({ length: ROWS }, (_, i) => ROWS - 1 - i);

/** Cuánto queda marcada en ámbar una columna llena después del clic (diseño 02b). */
const INVALID_MS = 1800;

@Component({
  selector: 'app-board',
  templateUrl: './board.html',
  styleUrl: './board.css',
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: {
    '(document:keydown)': 'onKeydown($event)',
  },
})
export class Board {
  readonly state = input.required<GameState>();
  /** Bloquea el tablero, por ejemplo mientras el bot piensa. */
  readonly disabled = input(false);

  /** Solo emite columnas incluidas en `legal_moves`. */
  readonly columnSelected = output<number>();

  private readonly hovered = signal<number | null>(null);
  private readonly invalid = signal<number | null>(null);
  /** Alterna entre dos animaciones iguales para que el temblor se repita en cada clic. */
  protected readonly shakeTurn = signal(0);
  private invalidTimer: ReturnType<typeof setTimeout> | undefined;

  /** Le toca al humano según el contrato: status playing y to_move === human. */
  protected readonly interactive = computed(() => {
    const state = this.state();
    return !this.disabled() && state.status === 'playing' && state.to_move === state.human;
  });

  protected readonly playing = computed(() => this.state().status === 'playing');

  protected readonly columns = computed<ColumnView[]>(() => {
    const state = this.state();
    const interactive = this.interactive();
    return Array.from({ length: COLUMNS }, (_, column) => {
      const legal = state.legal_moves.includes(column);
      const focused = interactive && legal && this.hovered() === column;
      const ghostRow = focused ? landingRow(state.board, column) : null;
      return {
        index: column,
        label: column + 1,
        legal,
        focused,
        invalid: this.invalid() === column,
        cells: ROWS_TOP_TO_BOTTOM.map((row) => ({
          row,
          owner: ownerOf(state, state.board[row][column]),
          ghost: row === ghostRow,
        })),
      };
    });
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.invalidTimer));
  }

  protected select(column: number): void {
    if (!this.interactive()) return;
    if (!this.state().legal_moves.includes(column)) {
      this.flagInvalid(column);
      return;
    }
    this.hovered.set(null);
    this.columnSelected.emit(column);
  }

  protected focus(column: number | null): void {
    this.hovered.set(column);
  }

  /** Teclas 1 a 8 eligen columna, como en el diseño. */
  protected onKeydown(event: KeyboardEvent): void {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    const column = Number.parseInt(event.key, 10) - 1;
    if (column >= 0 && column < COLUMNS) this.select(column);
  }

  private flagInvalid(column: number): void {
    this.invalid.set(column);
    this.shakeTurn.update((turn) => turn + 1);
    clearTimeout(this.invalidTimer);
    this.invalidTimer = setTimeout(() => this.invalid.set(null), INVALID_MS);
  }
}

function ownerOf(state: GameState, cell: Cell): Owner {
  if (cell === 0) return null;
  return cell === state.human ? 'human' : 'bot';
}
