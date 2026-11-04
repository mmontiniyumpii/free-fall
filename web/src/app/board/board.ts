import {
  ChangeDetectionStrategy,
  Component,
  DestroyRef,
  computed,
  inject,
  input,
  linkedSignal,
  output,
  signal,
} from '@angular/core';

import { COLUMNS, Cell, CellPosition, GameState, ROWS } from '../game/game-state';
import { landingRow } from '../game/landing-row';
import { sortWinningCells } from '../game/winning-line';

type Owner = 'human' | 'bot' | null;

interface CellView {
  row: number;
  owner: Owner;
  ghost: boolean;
  /** La ficha cae desde el carril (recién jugada). */
  dropping: boolean;
  /** Filas que recorre por encima de su casilla; da la distancia y la duración de la caída. */
  rowsAbove: number;
  /** Espera antes de caer, para que dos fichas nuevas caigan en orden. */
  dropDelayMs: number;
  /** Posición en la línea ganadora (0 a 3), o -1 si no es parte de ella. */
  winOrder: number;
  /** Partida ganada y esta ficha no es de la línea: se atenúa. */
  dimmed: boolean;
}

/**
 * Trazo que une los centros de la línea ganadora, en unidades de celda desde
 * la esquina superior izquierda de la grilla (fila 0 abajo ya convertida).
 */
interface Stroke {
  x: number;
  y: number;
  length: number;
  angleDeg: number;
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

/** Tablero de la pantalla de inicio, antes de crear la partida. */
const EMPTY_BOARD: Cell[][] = Array.from({ length: ROWS }, () => Array<Cell>(COLUMNS).fill(0));

/** Cuánto queda marcada en ámbar una columna llena después del clic (diseño 02b). */
const INVALID_MS = 1800;

/** Duración máxima de una caída (diseño: 220 ms + 22 ms por fila, tope 352 ms). */
const MAX_DROP_MS = 352;

interface Snapshot {
  state: GameState | null;
  pending: number | null;
}

/** Celda como "fila:columna" para comparar tableros. */
type CellKey = `${number}:${number}`;

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
  /** `null` en la pantalla de inicio: se dibuja vacío y sin interacción. */
  readonly state = input.required<GameState | null>();
  /** Bloquea el tablero, por ejemplo mientras el bot piensa. */
  readonly disabled = input(false);
  /** Jugada del humano enviada y sin respuesta: se dibuja ya, cayendo, sin tocar el GameState. */
  readonly pendingColumn = input<number | null>(null);
  /** Sin conexión con el bot: tablero atenuado y aviso en el carril. */
  readonly offline = input(false);

  /** Solo emite columnas incluidas en `legal_moves`. */
  readonly columnSelected = output<number>();
  /** Columna previsualizada (legal y con el tablero habilitado), o null. */
  readonly columnFocused = output<number | null>();
  /** Columna llena que se intentó jugar; vuelve a null cuando se apaga el aviso. */
  readonly columnRejected = output<number | null>();

  private readonly hovered = signal<number | null>(null);
  private readonly invalid = signal<number | null>(null);
  /** Alterna entre dos animaciones iguales para que el temblor se repita en cada clic. */
  protected readonly shakeTurn = signal(0);
  private invalidTimer: ReturnType<typeof setTimeout> | undefined;

  /** Le toca al humano según el contrato: status playing y to_move === human. */
  protected readonly interactive = computed(() => {
    const state = this.state();
    return (
      !this.disabled() && state?.status === 'playing' && state.to_move === state.human
    );
  });

  protected readonly playing = computed(() => this.state()?.status === 'playing');

  /** Dónde se dibuja la jugada pendiente; landingRow solo se usa para dibujar. */
  private readonly pendingCell = computed<CellKey | null>(() => {
    const state = this.state();
    const column = this.pendingColumn();
    if (!state || column === null) return null;
    const row = landingRow(state.board, column);
    return row === null ? null : `${row}:${column}`;
  });

  /**
   * Fichas que llegaron con el último GameState y todavía no se vieron caer,
   * en orden (primero la del humano). linkedSignal recuerda el valor anterior,
   * así se compara el tablero nuevo contra el que se estaba mostrando.
   */
  private readonly fresh = linkedSignal<Snapshot, CellKey[]>({
    source: () => ({ state: this.state(), pending: this.pendingColumn() }),
    computation: (current, previous) => freshCells(current, previous?.source ?? null),
  });

  /** Línea ganadora ordenada, solo si el backend la informó. */
  private readonly winLine = computed<CellPosition[]>(() => {
    const state = this.state();
    if (state?.status !== 'won' || state.winning_cells?.length !== 4) return [];
    return sortWinningCells(state.winning_cells);
  });

  protected readonly stroke = computed<Stroke | null>(() => {
    const line = this.winLine();
    if (line.length === 0) return null;
    const [firstRow, firstColumn] = line[0];
    const [lastRow, lastColumn] = line[line.length - 1];
    // En pantalla y crece hacia abajo: la fila del contrato se invierte acá.
    const dx = lastColumn - firstColumn;
    const dy = ROWS - 1 - lastRow - (ROWS - 1 - firstRow);
    return {
      x: firstColumn + 0.5,
      y: ROWS - 1 - firstRow + 0.5,
      length: Math.hypot(dx, dy),
      angleDeg: (Math.atan2(dy, dx) * 180) / Math.PI,
    };
  });

  protected readonly columns = computed<ColumnView[]>(() => {
    const state = this.state();
    const board = state?.board ?? EMPTY_BOARD;
    const interactive = this.interactive();
    const winLine = this.winLine();
    return Array.from({ length: COLUMNS }, (_, column) => {
      const legal = this.isLegal(column);
      const focused = interactive && legal && this.hovered() === column;
      const ghostRow = focused ? landingRow(board, column) : null;
      const pending = this.pendingCell();
      const fresh = this.fresh();
      return {
        index: column,
        label: column + 1,
        legal,
        focused,
        invalid: this.invalid() === column,
        cells: ROWS_TOP_TO_BOTTOM.map((row) => {
          const key: CellKey = `${row}:${column}`;
          const isPending = key === pending;
          const order = fresh.indexOf(key);
          const owner = isPending ? 'human' : state ? ownerOf(state, board[row][column]) : null;
          const winOrder = winLine.findIndex(([r, c]) => r === row && c === column);
          return {
            row,
            owner,
            winOrder,
            dimmed: winLine.length > 0 && owner !== null && winOrder < 0,
            ghost: row === ghostRow,
            dropping: isPending || order >= 0,
            rowsAbove: ROWS - 1 - row,
            dropDelayMs: Math.max(order, 0) * MAX_DROP_MS,
          };
        }),
      };
    });
  });

  constructor() {
    inject(DestroyRef).onDestroy(() => clearTimeout(this.invalidTimer));
  }

  protected select(column: number): void {
    if (!this.interactive()) return;
    if (!this.isLegal(column)) {
      this.flagInvalid(column);
      return;
    }
    this.focus(null);
    this.columnSelected.emit(column);
  }

  protected focus(column: number | null): void {
    this.hovered.set(column);
    const shown = column !== null && this.interactive() && this.isLegal(column) ? column : null;
    this.columnFocused.emit(shown);
  }

  private isLegal(column: number): boolean {
    return this.state()?.legal_moves.includes(column) ?? false;
  }

  /** Teclas 1 a 8 eligen columna, como en el diseño. */
  protected onKeydown(event: KeyboardEvent): void {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    const column = Number.parseInt(event.key, 10) - 1;
    if (column >= 0 && column < COLUMNS) this.select(column);
  }

  private flagInvalid(column: number): void {
    this.invalid.set(column);
    this.columnRejected.emit(column);
    this.shakeTurn.update((turn) => turn + 1);
    clearTimeout(this.invalidTimer);
    this.invalidTimer = setTimeout(() => {
      this.invalid.set(null);
      this.columnRejected.emit(null);
    }, INVALID_MS);
  }
}

/**
 * Celdas ocupadas en el estado actual que estaban vacías en el que se mostraba
 * antes. No interpreta reglas: solo compara tableros. Excluye la jugada
 * pendiente que ya se vio caer. Una partida nueva solo anima si trae a lo sumo
 * una ficha (la apertura del bot); cargar una partida empezada no anima nada.
 */
function freshCells(current: Snapshot, previous: Snapshot | null): CellKey[] {
  const state = current.state;
  if (!state) return [];
  const before = previous?.state;
  const sameGame = before?.id === state.id && before.history.length <= state.history.length;
  const baseline = sameGame ? before.board : state.history.length <= 1 ? EMPTY_BOARD : null;
  if (!baseline) return [];

  const alreadyShown =
    sameGame && previous?.pending != null ? landingRow(before.board, previous.pending) : null;
  const cells: { key: CellKey; human: boolean }[] = [];
  state.board.forEach((rowCells, row) =>
    rowCells.forEach((cell, column) => {
      if (cell === 0 || baseline[row][column] !== 0) return;
      if (alreadyShown === row && previous?.pending === column) return;
      cells.push({ key: `${row}:${column}`, human: cell === state.human });
    }),
  );
  // El humano siempre juega antes que la respuesta del bot.
  return cells.sort((a, b) => Number(b.human) - Number(a.human)).map((c) => c.key);
}

function ownerOf(state: GameState, cell: Cell): Owner {
  if (cell === 0) return null;
  return cell === state.human ? 'human' : 'bot';
}
