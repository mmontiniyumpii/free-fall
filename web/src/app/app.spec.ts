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

  it('al hacer clic en una columna la ficha aparece abajo', async () => {
    vi.useFakeTimers();
    const fixture = TestBed.createComponent(App);
    await vi.advanceTimersByTimeAsync(0);
    fixture.detectChanges();
    const host = fixture.nativeElement as HTMLElement;
    const column0 = host.querySelectorAll<HTMLElement>('app-board .column')[0];

    column0.click();
    await vi.advanceTimersByTimeAsync(1000);
    fixture.detectChanges();

    const cells = column0.querySelectorAll('.cell');
    expect(cells[cells.length - 1].querySelector('.disc--human')).not.toBeNull();
    vi.useRealTimers();
  });
});
