import { TestBed } from '@angular/core/testing';

import { App } from './app';
import { FAKE_SCENARIO, FakeGameApiService } from './game/fake-game-api.service';
import { GameApi } from './game/game-api';

describe('App', () => {
  beforeEach(async () => {
    await TestBed.configureTestingModule({
      imports: [App],
      providers: [
        { provide: GameApi, useClass: FakeGameApiService },
        { provide: FAKE_SCENARIO, useValue: 'playing' },
      ],
    }).compileComponents();
  });

  it('muestra el tablero con la partida que devuelve la API', async () => {
    const fixture = TestBed.createComponent(App);
    await fixture.whenStable();
    const host = fixture.nativeElement as HTMLElement;

    expect(host.querySelector('.brand')?.textContent).toContain('Cuatro en línea');
    expect(host.querySelectorAll('app-board .cell').length).toBe(56);
    // El fixture tiene 6 fichas: 4 en la columna 3 y 2 en la columna 4.
    expect(host.querySelectorAll('app-board .disc').length).toBe(6);
  });
});
