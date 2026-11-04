import { TestBed } from '@angular/core/testing';

import { fakeState } from '../game/fake-scenarios';
import { Cell, GameState } from '../game/game-state';
import { Board } from './board';

type Owner = 'human' | 'bot' | null;

function setup(state: GameState, disabled = false) {
  const fixture = TestBed.createComponent(Board);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('disabled', disabled);
  const selected: number[] = [];
  fixture.componentInstance.columnSelected.subscribe((column) => selected.push(column));
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const column = (index: number) => host.querySelectorAll<HTMLElement>('.column')[index];
  return { fixture, host, selected, column };
}

function render(state: GameState): HTMLElement {
  return setup(state).host;
}

/** Fixture con la columna 0 llena (sin cuatro seguidas) y fuera de legal_moves. */
function withFullColumnZero(): GameState {
  const state = fakeState('playing');
  state.board.forEach((row, index) => (row[0] = index % 2 === 0 ? 1 : 2));
  state.legal_moves = [1, 2, 3, 4, 5, 6, 7];
  return state;
}

/** Dueño de cada celda de una columna, en el orden del DOM (de arriba hacia abajo). */
function columnTopToBottom(host: HTMLElement, column: number): Owner[] {
  const columns = host.querySelectorAll('.column');
  return Array.from(columns[column].querySelectorAll('.cell'), (cell) => {
    if (cell.querySelector('.disc--human')) return 'human';
    if (cell.querySelector('.disc--bot')) return 'bot';
    return null;
  });
}

function emptyBoard(): Cell[][] {
  return Array.from({ length: 7 }, () => Array<Cell>(8).fill(0));
}

describe('Board', () => {
  it('dibuja 8 columnas de 7 celdas, numeradas 1 a 8 de izquierda a derecha', () => {
    const host = render(fakeState('playing'));

    expect(host.querySelectorAll('.column').length).toBe(8);
    expect(host.querySelectorAll('.cell').length).toBe(56);
    expect(Array.from(host.querySelectorAll('.label'), (l) => l.textContent?.trim())).toEqual([
      '1', '2', '3', '4', '5', '6', '7', '8',
    ]);
  });

  it('dibuja la fila 0 abajo: board[0][3] es la última celda de la columna 3 en el DOM', () => {
    // Fixture: columna 3 con 1, 1, 1, 2 desde abajo; columna 4 con 2, 2.
    const host = render(fakeState('playing'));

    expect(columnTopToBottom(host, 3)).toEqual([null, null, null, 'bot', 'human', 'human', 'human']);
    expect(columnTopToBottom(host, 4)).toEqual([null, null, null, null, null, 'bot', 'bot']);
    expect(columnTopToBottom(host, 0)).toEqual([null, null, null, null, null, null, null]);
  });

  it('una ficha en la fila 6 queda arriba de todo', () => {
    const board = emptyBoard();
    board[6][7] = 1;
    const host = render({ ...fakeState('playing'), board });

    expect(columnTopToBottom(host, 7)).toEqual(['human', null, null, null, null, null, null]);
  });

  it('distingue humano y bot por los campos human/bot, no por el número de jugador', () => {
    const board = emptyBoard();
    board[0][0] = 2;
    board[0][1] = 1;
    const host = render({ ...fakeState('playing'), board, human: 2, bot: 1 });

    expect(columnTopToBottom(host, 0).at(-1)).toBe('human');
    expect(columnTopToBottom(host, 1).at(-1)).toBe('bot');
  });

  describe('interacción', () => {
    it('un clic en una columna legal emite su índice', () => {
      const { column, selected } = setup(fakeState('playing'));

      column(5).click();

      expect(selected).toEqual([5]);
    });

    it('un clic en una columna llena no emite nada y la marca como inválida', () => {
      const { fixture, column, selected, host } = setup(withFullColumnZero());

      column(0).click();
      fixture.detectChanges();

      expect(selected).toEqual([]);
      expect(column(0).classList).toContain('column--invalid');
      expect(host.querySelector('.full--invalid')?.textContent).toContain('Llena');
    });

    it('"Llena" aparece solo sobre las columnas que no están en legal_moves', () => {
      const host = render(withFullColumnZero());

      const slots = Array.from(host.querySelectorAll('.slot'), (slot) => slot.textContent?.trim());
      expect(slots).toEqual(['Llena', '', '', '', '', '', '', '']);
    });

    it('bloqueado (disabled) no emite y no muestra avisos', () => {
      const { column, selected, host } = setup(withFullColumnZero(), true);

      column(5).click();

      expect(selected).toEqual([]);
      expect(host.querySelector('.full')).toBeNull();
    });

    it('si no le toca al humano no emite', () => {
      const { column, selected } = setup({ ...fakeState('playing'), to_move: 2 });

      column(5).click();

      expect(selected).toEqual([]);
    });

    it('con la partida terminada no emite ni muestra "Llena"', () => {
      const { column, selected, host } = setup(fakeState('won'));

      column(0).click();

      expect(selected).toEqual([]);
      expect(host.querySelector('.full')).toBeNull();
    });

    it('las teclas 1 a 8 eligen columna; otras teclas o con modificador no', () => {
      const { selected } = setup(fakeState('playing'));

      document.dispatchEvent(new KeyboardEvent('keydown', { key: '4' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '9' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '0' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '2', ctrlKey: true }));

      expect(selected).toEqual([3]);
    });

    it('al pasar el mouse muestra la ficha en el carril y el contorno donde cae', () => {
      const { fixture, column, host } = setup(fakeState('playing'));

      column(3).dispatchEvent(new MouseEvent('mouseenter'));
      fixture.detectChanges();

      expect(host.querySelectorAll('.slot')[3].querySelector('.preview')).not.toBeNull();
      // Columna 3 del fixture: fichas en las filas 0 a 3, cae en la fila 4,
      // que es la tercera celda desde arriba en el DOM.
      const ghosts = Array.from(column(3).querySelectorAll('.cell'), (cell) => !!cell.querySelector('.ghost'));
      expect(ghosts).toEqual([false, false, true, false, false, false, false]);
    });
  });
});
