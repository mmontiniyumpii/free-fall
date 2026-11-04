import { TestBed } from '@angular/core/testing';

import { fakeState } from '../game/fake-scenarios';
import { Cell, GameState } from '../game/game-state';
import { Board } from './board';

type Owner = 'human' | 'bot' | null;

function render(state: GameState): HTMLElement {
  const fixture = TestBed.createComponent(Board);
  fixture.componentRef.setInput('state', state);
  fixture.detectChanges();
  return fixture.nativeElement as HTMLElement;
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
});
