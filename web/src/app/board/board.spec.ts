import { TestBed } from '@angular/core/testing';

import { fakeState } from '../game/fake-scenarios';
import { Cell, GameState } from '../game/game-state';
import { Board } from './board';

type Owner = 'human' | 'bot' | null;

function setup(state: GameState | null, disabled = false) {
  const fixture = TestBed.createComponent(Board);
  fixture.componentRef.setInput('state', state);
  fixture.componentRef.setInput('disabled', disabled);
  const selected: number[] = [];
  const focused: (number | null)[] = [];
  const rejected: (number | null)[] = [];
  fixture.componentInstance.columnSelected.subscribe((column) => selected.push(column));
  fixture.componentInstance.columnFocused.subscribe((column) => focused.push(column));
  fixture.componentInstance.columnRejected.subscribe((column) => rejected.push(column));
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const column = (index: number) => host.querySelectorAll<HTMLElement>('.column')[index];
  return { fixture, host, selected, focused, rejected, column };
}

function render(state: GameState): HTMLElement {
  return setup(state).host;
}

/** Fixture con la columna 0 llena (sin cuatro seguidas) y fuera de legal_moves. */
function withFullColumnZero(): GameState {
  const state = fakeState('fixture');
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
    const host = render(fakeState('fixture'));

    expect(host.querySelectorAll('.column').length).toBe(8);
    expect(host.querySelectorAll('.cell').length).toBe(56);
    expect(Array.from(host.querySelectorAll('.label'), (l) => l.textContent?.trim())).toEqual([
      '1', '2', '3', '4', '5', '6', '7', '8',
    ]);
  });

  it('dibuja la fila 0 abajo: board[0][3] es la última celda de la columna 3 en el DOM', () => {
    // Fixture: columna 3 con 1, 1, 1, 2 desde abajo; columna 4 con 2, 2.
    const host = render(fakeState('fixture'));

    expect(columnTopToBottom(host, 3)).toEqual([null, null, null, 'bot', 'human', 'human', 'human']);
    expect(columnTopToBottom(host, 4)).toEqual([null, null, null, null, null, 'bot', 'bot']);
    expect(columnTopToBottom(host, 0)).toEqual([null, null, null, null, null, null, null]);
  });

  it('una ficha en la fila 6 queda arriba de todo', () => {
    const board = emptyBoard();
    board[6][7] = 1;
    const host = render({ ...fakeState('fixture'), board });

    expect(columnTopToBottom(host, 7)).toEqual(['human', null, null, null, null, null, null]);
  });

  it('distingue humano y bot por los campos human/bot, no por el número de jugador', () => {
    const board = emptyBoard();
    board[0][0] = 2;
    board[0][1] = 1;
    const host = render({ ...fakeState('fixture'), board, human: 2, bot: 1 });

    expect(columnTopToBottom(host, 0).at(-1)).toBe('human');
    expect(columnTopToBottom(host, 1).at(-1)).toBe('bot');
  });

  describe('interacción', () => {
    it('un clic en una columna legal emite su índice', () => {
      const { column, selected } = setup(fakeState('fixture'));

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
      const { column, selected } = setup({ ...fakeState('fixture'), to_move: 2 });

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
      const { selected } = setup(fakeState('fixture'));

      document.dispatchEvent(new KeyboardEvent('keydown', { key: '4' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '9' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '0' }));
      document.dispatchEvent(new KeyboardEvent('keydown', { key: '2', ctrlKey: true }));

      expect(selected).toEqual([3]);
    });

    it('al pasar el mouse muestra la ficha en el carril y el contorno donde cae', () => {
      const { fixture, column, host } = setup(fakeState('fixture'));

      column(3).dispatchEvent(new MouseEvent('mouseenter'));
      fixture.detectChanges();

      expect(host.querySelectorAll('.slot')[3].querySelector('.preview')).not.toBeNull();
      // Columna 3 del fixture: fichas en las filas 0 a 3, cae en la fila 4,
      // que es la tercera celda desde arriba en el DOM.
      const ghosts = Array.from(column(3).querySelectorAll('.cell'), (cell) => !!cell.querySelector('.ghost'));
      expect(ghosts).toEqual([false, false, true, false, false, false, false]);
    });
  });

  describe('inicio y avisos al panel de estado', () => {
    it('sin partida (pantalla de inicio) dibuja el tablero vacío y no responde', () => {
      const { host, column, selected } = setup(null);

      expect(host.querySelectorAll('.cell').length).toBe(56);
      expect(host.querySelector('.disc')).toBeNull();
      column(2).click();
      expect(selected).toEqual([]);
    });

    it('avisa qué columna se previsualiza y cuándo se deja de previsualizar', () => {
      const { column, host, focused } = setup(fakeState('fixture'));

      column(5).dispatchEvent(new MouseEvent('mouseenter'));
      host.querySelector('.board')?.dispatchEvent(new MouseEvent('mouseleave'));

      expect(focused).toEqual([5, null]);
    });

    it('avisa la columna llena rechazada y la limpia cuando se apaga el aviso', async () => {
      vi.useFakeTimers();
      const { column, rejected } = setup(withFullColumnZero());

      column(0).click();
      expect(rejected).toEqual([0]);
      await vi.advanceTimersByTimeAsync(2000);
      expect(rejected).toEqual([0, null]);
      vi.useRealTimers();
    });
  });

  describe('caída de fichas', () => {
    function drops(host: HTMLElement): string[] {
      return Array.from(host.querySelectorAll<HTMLElement>('.disc--drop'), (disc) => {
        const cell = disc.closest('.cell') as HTMLElement;
        const column = (disc.closest('.column') as HTMLElement).dataset['column'];
        const kind = disc.classList.contains('disc--human') ? 'human' : 'bot';
        return `${kind}@${cell.dataset['row']}:${column}`;
      });
    }

    function withMove(state: GameState, row: number, column: number, player: 1 | 2): GameState {
      const board = state.board.map((cells) => [...cells]);
      board[row][column] = player;
      return { ...state, board, history: [...state.history, column] };
    }

    it('la jugada pendiente del humano se dibuja ya, cayendo, donde cae', () => {
      const { fixture, host } = setup(fakeState('fixture'));

      fixture.componentRef.setInput('pendingColumn', 3);
      fixture.detectChanges();

      // Columna 3 del fixture: cae en la fila 4.
      expect(drops(host)).toEqual(['human@4:3']);
    });

    it('si la respuesta llega mientras tu ficha cae, la caída sigue sin reiniciarse y después cae la del bot', () => {
      const before = fakeState('fixture');
      const { fixture, host } = setup(before);
      fixture.componentRef.setInput('pendingColumn', 3);
      fixture.detectChanges();
      const falling = host.querySelector('.disc--human.disc--drop');

      const afterHuman = withMove(before, 4, 3, 1);
      fixture.componentRef.setInput('state', withMove(afterHuman, 2, 4, 2));
      fixture.componentRef.setInput('pendingColumn', null);
      fixture.detectChanges();

      // Mismo elemento y sigue con la caída: la animación en curso no se corta.
      expect(host.querySelector('.disc--human.disc--drop')).toBe(falling);
      expect(drops(host).sort()).toEqual(['bot@2:4', 'human@4:3']);
      const botDelay = host.querySelector<HTMLElement>('.disc--bot.disc--drop')?.style.getPropertyValue('--drop-delay');
      expect(botDelay).not.toBe('0ms');
    });

    it('si llegan dos fichas nuevas de golpe, cae primero la del humano', () => {
      const before = fakeState('fixture');
      const { fixture, host } = setup(before);

      fixture.componentRef.setInput('state', withMove(withMove(before, 4, 3, 1), 2, 4, 2));
      fixture.detectChanges();

      expect(drops(host).sort()).toEqual(['bot@2:4', 'human@4:3']);
      const delays = Array.from(host.querySelectorAll<HTMLElement>('.disc--drop'), (disc) => ({
        human: disc.classList.contains('disc--human'),
        delay: disc.style.getPropertyValue('--drop-delay'),
      }));
      expect(delays.find((d) => d.human)?.delay).toBe('0ms');
      expect(delays.find((d) => !d.human)?.delay).not.toBe('0ms');
    });

    it('cargar una partida empezada no hace caer nada', () => {
      expect(drops(render(fakeState('fixture')))).toEqual([]);
    });

    it('la apertura del bot en una partida nueva cae', () => {
      const { fixture, host } = setup(null);

      fixture.componentRef.setInput('state', fakeState('playing', 'bot'));
      fixture.detectChanges();

      expect(drops(host)).toEqual(['bot@0:3']);
    });
  });

  it('sin conexión atenúa el tablero y lo avisa en el carril', () => {
    const { fixture, host } = setup(fakeState('fixture'));

    fixture.componentRef.setInput('offline', true);
    fixture.detectChanges();

    expect(host.querySelector('.board--offline')).not.toBeNull();
    expect(host.querySelector('.offline')?.textContent).toContain('Sin conexión con el bot');
  });

  describe('fin de partida', () => {
    /** Anillos como "fila:columna" en el orden de la línea. */
    function rings(host: HTMLElement): string[] {
      return Array.from(host.querySelectorAll<HTMLElement>('.ring'))
        .map((ring) => {
          const cell = ring.closest('.cell') as HTMLElement;
          const column = (ring.closest('.column') as HTMLElement).dataset['column'];
          return { order: Number(ring.style.getPropertyValue('--win-order')), key: `${cell.dataset['row']}:${column}` };
        })
        .sort((a, b) => a.order - b.order)
        .map((ring) => ring.key);
    }

    function stroke(host: HTMLElement) {
      const element = host.querySelector<HTMLElement>('.stroke');
      if (!element) return null;
      const value = (name: string) => Number.parseFloat(element.style.getPropertyValue(name));
      return {
        x: value('--x'),
        y: value('--y'),
        length: Number(value('--length').toFixed(3)),
        angle: Math.round(value('--angle')),
      };
    }

    it('marca con anillos las celdas de winning_cells, con la fila 0 abajo', () => {
      const host = render(fakeState('won'));

      expect(rings(host)).toEqual(['0:2', '1:3', '2:4', '3:5']);
      // En el DOM, la celda (0,2) es la última de la columna 2.
      const column2 = host.querySelectorAll('.column')[2].querySelectorAll('.cell');
      expect(column2[column2.length - 1].querySelector('.ring')).not.toBeNull();
    });

    it('anima los anillos en el orden de la línea aunque winning_cells venga desordenado', () => {
      const state = fakeState('won');
      state.winning_cells = [[3, 5], [1, 3], [0, 2], [2, 4]];

      expect(rings(render(state))).toEqual(['0:2', '1:3', '2:4', '3:5']);
    });

    it('atenúa todas las fichas que no son de la línea', () => {
      const host = render(fakeState('won'));

      // El tablero ganado tiene 11 fichas, 4 en la línea.
      expect(host.querySelectorAll('.disc').length).toBe(11);
      expect(host.querySelectorAll('.disc--dim').length).toBe(7);
      expect(host.querySelector('.ring + .hole .disc--dim')).toBeNull();
    });

    it('el trazo de una diagonal ascendente sube hacia la derecha', () => {
      // De (0,2) a (3,5): en pantalla, de la última fila hacia arriba a 45°.
      expect(stroke(render(fakeState('won')))).toEqual({ x: 2.5, y: 6.5, length: 4.243, angle: -45 });
    });

    it('el trazo de una vertical sube desde la fila 0', () => {
      expect(stroke(render(fakeState('bot_won')))).toEqual({ x: 6.5, y: 6.5, length: 3, angle: -90 });
    });

    it('el trazo de una horizontal es recto', () => {
      const state = fakeState('won');
      state.winning_cells = [[2, 1], [2, 2], [2, 3], [2, 4]];

      expect(stroke(render(state))).toEqual({ x: 1.5, y: 4.5, length: 3, angle: 0 });
    });

    it('en empate no hay anillos, trazo ni fichas atenuadas', () => {
      const host = render(fakeState('draw'));

      expect(host.querySelector('.ring')).toBeNull();
      expect(host.querySelector('.stroke')).toBeNull();
      expect(host.querySelector('.disc--dim')).toBeNull();
    });
  });

  describe('animación final', () => {
    function ending(host: HTMLElement): string | null {
      const board = host.querySelector('.board') as HTMLElement;
      return ['won', 'lost', 'draw'].find((kind) => board.classList.contains(`board--${kind}`)) ?? null;
    }

    it('ganar y perder tienen cada uno su animación', () => {
      expect(ending(render(fakeState('won')))).toBe('won');
      expect(ending(render(fakeState('bot_won')))).toBe('lost');
    });

    it('empate: la ola recorre las columnas en orden', () => {
      const host = render(fakeState('draw'));

      expect(ending(host)).toBe('draw');
      const order = Array.from(host.querySelectorAll<HTMLElement>('.column'), (c) =>
        c.style.getPropertyValue('--column'),
      );
      expect(order).toEqual(['0', '1', '2', '3', '4', '5', '6', '7']);
    });

    it('durante la partida no hay animación final', () => {
      const host = render(fakeState('fixture'));

      expect(ending(host)).toBeNull();
    });

    it('si gana el humano siendo el jugador 2, también es victoria', () => {
      const state = { ...fakeState('bot_won'), human: 2 as const, bot: 1 as const };

      expect(ending(render(state))).toBe('won');
    });
  });
});
