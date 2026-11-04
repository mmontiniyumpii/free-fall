import { TestBed } from '@angular/core/testing';

import { fakeState } from '../game/fake-scenarios';
import { GameState } from '../game/game-state';
import { Status } from './status';

interface Inputs {
  thinking?: boolean;
  focusedColumn?: number | null;
  rejectedColumn?: number | null;
  retryIn?: number | null;
  notice?: { title: string; detail: string } | null;
}

function render(state: GameState | null, inputs: Inputs = {}) {
  const fixture = TestBed.createComponent(Status);
  fixture.componentRef.setInput('state', state);
  for (const [name, value] of Object.entries(inputs)) fixture.componentRef.setInput(name, value);
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const text = (selector: string) => host.querySelector(selector)?.textContent?.trim() ?? null;
  return {
    host,
    eyebrow: text('.eyebrow'),
    title: text('.title'),
    detail: text('.detail'),
    turn: host.querySelector('.turn') ? (host.querySelector('.turn--bot') ? 'bot' : 'human') : null,
  };
}

describe('Status', () => {
  it('en el inicio presenta el juego, sin indicador de turno', () => {
    const view = render(null);

    expect(view.eyebrow).toBe('Nueva partida');
    expect(view.title).toBe('Cuatro en línea');
    expect(view.turn).toBeNull();
  });

  it('turno del humano: número de jugada, "Tu turno" e indicador en Vos', () => {
    const view = render(fakeState('fixture'));

    expect(view.eyebrow).toBe('Jugada 7');
    expect(view.title).toBe('Tu turno');
    expect(view.detail).toBe('Elegí una columna.');
    expect(view.turn).toBe('human');
  });

  it('al previsualizar una columna dice dónde cae, con filas contadas desde abajo', () => {
    expect(render(fakeState('fixture'), { focusedColumn: 5 }).detail).toBe(
      'Columna 6 · cae en la fila 1.',
    );
    // Columna 3 del fixture: cuatro fichas, la próxima cae en la quinta fila.
    expect(render(fakeState('fixture'), { focusedColumn: 3 }).detail).toBe(
      'Columna 4 · cae en la fila 5.',
    );
  });

  it('una columna llena se avisa en el detalle', () => {
    const view = render(fakeState('fixture'), { rejectedColumn: 0, focusedColumn: 5 });

    expect(view.detail).toBe('La columna 1 está llena. Elegí otra.');
    expect(view.host.querySelector('.detail--warning')).not.toBeNull();
  });

  it('mientras el bot piensa lo dice y el indicador pasa al bot', () => {
    const view = render(fakeState('fixture'), { thinking: true });

    expect(view.title).toBe('El bot está pensando');
    expect(view.turn).toBe('bot');
    expect(view.host.querySelector('.dots')).not.toBeNull();
  });

  it('si le toca al bot sin pedido en curso, muestra que es su turno', () => {
    expect(render({ ...fakeState('fixture'), to_move: 2 }).title).toBe('Turno del bot');
  });

  it('fin de partida: ganador según human/winner, o empate', () => {
    expect(render(fakeState('won')).title).toBe('Ganaste');
    expect(render({ ...fakeState('won'), human: 2, bot: 1 }).title).toBe('Ganó el bot');
    const draw = render(fakeState('draw'));
    expect(draw.title).toBe('Empate');
    expect(draw.eyebrow).toBe('Fin de la partida · 56 jugadas');
    expect(draw.turn).toBeNull();
  });

  it('sin conexión: título de alerta y cuenta regresiva del reintento', () => {
    const view = render(fakeState('fixture'), { retryIn: 3 });

    expect(view.title).toBe('Sin conexión');
    expect(view.detail).toBe('No se pudo conectar con el bot. Reintentamos en 3 s.');
    expect(view.host.querySelector('.title--alert')).not.toBeNull();
  });

  it('un aviso (partida perdida) reemplaza al estado normal', () => {
    const view = render(fakeState('fixture'), {
      notice: { title: 'La partida ya no existe', detail: 'Empezá una nueva.' },
    });

    expect(view.title).toBe('La partida ya no existe');
    expect(view.detail).toBe('Empezá una nueva.');
    expect(view.turn).toBeNull();
  });

  it('al ganar o perder describe la línea con columnas contadas desde 1', () => {
    expect(render(fakeState('won')).detail).toBe(
      'Conectaste cuatro en diagonal, de la columna 3 a la 6.',
    );
    const lost = render(fakeState('bot_won'));
    expect(lost.title).toBe('Ganó el bot');
    expect(lost.detail).toBe('Conectó cuatro en vertical, en la columna 7.');
    const horizontal = fakeState('won');
    horizontal.winning_cells = [[2, 4], [2, 1], [2, 3], [2, 2]];
    expect(render(horizontal).detail).toBe(
      'Conectaste cuatro en horizontal, de la columna 2 a la 5.',
    );
  });
});
