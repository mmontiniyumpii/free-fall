import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';

import { GameState } from '../game/game-state';
import { ArbiterGame } from './arbiter-game';
import { ARBITER_BASE } from './arbiter-record.service';
import { Replay } from './replay';
import mockRecord from './testdata/game-record.mock.json';

const record = mockRecord as unknown as ArbiterGame;

function setup() {
  TestBed.configureTestingModule({
    providers: [provideHttpClient(), provideHttpClientTesting(), { provide: ARBITER_BASE, useValue: '/arbitro' }],
  });
  const fixture = TestBed.createComponent(Replay);
  const frames: (GameState | null)[] = [];
  fixture.componentInstance.frameChange.subscribe((frame) => frames.push(frame));
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const http = TestBed.inject(HttpTestingController);
  const button = (label: string) =>
    Array.from(host.querySelectorAll('button')).find(
      (b) => b.getAttribute('aria-label') === label || b.textContent?.trim() === label,
    ) as HTMLButtonElement;
  const search = (text: string) => {
    const input = host.querySelector('input#game-ref') as HTMLInputElement;
    input.value = text;
    input.dispatchEvent(new Event('input'));
    (host.querySelector('form') as HTMLFormElement).dispatchEvent(new Event('submit'));
    fixture.detectChanges();
  };
  const last = () => frames.at(-1) ?? null;
  const render = () => {
    fixture.detectChanges();
    TestBed.tick();
  };
  return { fixture, host, http, button, search, last, render };
}

describe('Replay', () => {
  it('carga la partida por código a través del proxy del árbitro y arranca en el tablero vacío', () => {
    const { http, search, last, render, host } = setup();

    search('https://arbitro.example/api/games/g_f_01');
    http.expectOne('/arbitro/api/games/g_f_01').flush(record);
    render();

    expect(last()?.history).toEqual([]);
    expect(host.querySelector('.step-title')?.textContent).toContain('Tablero vacío');
    expect(Array.from(host.querySelectorAll('.teams li'), (li) => li.textContent?.trim())).toEqual([
      'Equipo 1',
      'Equipo 2',
    ]);
    http.verify();
  });

  it('avanza, retrocede y salta al final, donde muestra el resultado del árbitro', () => {
    const { http, search, last, render, button, host } = setup();
    search('g_f_01');
    http.expectOne('/arbitro/api/games/g_f_01').flush(record);
    render();

    button('Jugada siguiente').click();
    render();
    button('Jugada siguiente').click();
    render();
    expect(last()?.history).toEqual([record.moves[0].column, record.moves[1].column]);

    button('Jugada anterior').click();
    render();
    expect(last()?.history.length).toBe(1);

    button('Ir al final').click();
    render();
    expect(last()?.status).toBe('won');
    expect(host.querySelector('.step-result')?.textContent).toContain('Ganó Equipo 2 con cuatro en línea.');
    expect(button('Jugada siguiente').disabled).toBe(true);
  });

  it('con las flechas del teclado avanza y retrocede', () => {
    const { http, search, last, render } = setup();
    search('g_f_01');
    http.expectOne('/arbitro/api/games/g_f_01').flush(record);
    render();

    const right = new KeyboardEvent('keydown', { key: 'ArrowRight', cancelable: true });
    document.dispatchEvent(right);
    render();
    expect(last()?.history.length).toBe(1);
    // La flecha no debe además desplazar el panel.
    expect(right.defaultPrevented).toBe(true);
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'ArrowLeft' }));
    render();
    expect(last()?.history.length).toBe(0);
  });

  it('avisa si la partida no existe o si el código no es válido', () => {
    const { http, search, render, host } = setup();

    search('esto no');
    expect(host.querySelector('.error')?.textContent).toContain('Escribí el código de la partida');

    search('g_nada');
    http
      .expectOne('/arbitro/api/games/g_nada')
      .flush({ ok: false, error: 'unknown_game', message: 'La partida g_nada no existe.' }, { status: 404, statusText: 'Not Found' });
    render();
    expect(host.querySelector('.error')?.textContent).toContain('No existe una partida con ese código.');
  });

  it('sin conexión con el árbitro lo dice', () => {
    const { http, search, render, host } = setup();

    search('g_f_01');
    http.expectOne('/arbitro/api/games/g_f_01').flush(null, { status: 0, statusText: 'Unknown Error' });
    render();

    expect(host.querySelector('.error')?.textContent).toContain('No se pudo conectar con el árbitro');
  });
});
