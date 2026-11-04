import { TestBed } from '@angular/core/testing';

import { fakeState } from '../game/fake-scenarios';
import { GameState } from '../game/game-state';
import { History } from './history';

function render(state: GameState) {
  const fixture = TestBed.createComponent(History);
  fixture.componentRef.setInput('state', state);
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  return {
    host,
    count: host.querySelector('.count')?.textContent?.trim(),
    rows: Array.from(host.querySelectorAll('.row'), (row) =>
      Array.from(row.querySelectorAll('.number, .who, .column'), (cell) => cell.textContent?.trim()).join(' '),
    ),
  };
}

describe('History', () => {
  it('lista las jugadas de la más nueva a la más vieja, con quién y en qué columna', () => {
    const view = render(fakeState('fixture'));

    expect(view.count).toBe('6 jugadas');
    expect(view.rows).toEqual([
      '06 Bot Columna 4',
      '05 Vos Columna 4',
      '04 Bot Columna 5',
      '03 Vos Columna 4',
      '02 Bot Columna 5',
      '01 Vos Columna 4',
    ]);
  });

  it('resalta solo la última jugada', () => {
    const { host } = render(fakeState('fixture'));

    const rows = host.querySelectorAll('.row');
    expect(rows[0].classList).toContain('row--last');
    expect(host.querySelectorAll('.row--last').length).toBe(1);
  });

  it('una jugada en singular y la apertura del bot como suya', () => {
    const view = render(fakeState('playing', 'bot'));

    expect(view.count).toBe('1 jugada');
    expect(view.rows).toEqual(['01 Bot Columna 4']);
  });

  it('sin jugadas lo dice', () => {
    const view = render(fakeState('playing', 'human'));

    expect(view.count).toBe('0 jugadas');
    expect(view.host.querySelector('.empty')?.textContent).toContain('Todavía no hay jugadas.');
  });
});
