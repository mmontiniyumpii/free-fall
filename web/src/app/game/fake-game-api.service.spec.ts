import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';

import { FAKE_SCENARIO, FakeGameApiService } from './fake-game-api.service';
import { FakeScenario } from './fake-scenarios';

function fakeApi(scenario: FakeScenario): FakeGameApiService {
  TestBed.configureTestingModule({
    providers: [FakeGameApiService, { provide: FAKE_SCENARIO, useValue: scenario }],
  });
  return TestBed.inject(FakeGameApiService);
}

describe('FakeGameApiService', () => {
  afterEach(() => vi.useRealTimers());

  it('playing: crea la partida del fixture del contrato', async () => {
    const state = await firstValueFrom(fakeApi('playing').create('human'));

    expect(state.id).toBe('l_0001');
    expect(state.status).toBe('playing');
    expect(state.to_move).toBe(1);
  });

  it('devuelve la dificultad con la que se pidió la partida', async () => {
    const state = await firstValueFrom(fakeApi('playing').create('human', 'easy'));

    expect(state.difficulty).toBe('easy');
  });

  it('won: partida ganada con las cuatro celdas de la línea', async () => {
    const state = await firstValueFrom(fakeApi('won').create('human'));

    expect(state.status).toBe('won');
    expect(state.winner).toBe(1);
    expect(state.winning_cells).toEqual([[0, 2], [1, 3], [2, 4], [3, 5]]);
  });

  it('draw: tablero lleno y sin ganador', async () => {
    const state = await firstValueFrom(fakeApi('draw').create('human'));

    expect(state.status).toBe('draw');
    expect(state.winner).toBeNull();
    expect(state.board.flat()).not.toContain(0);
  });

  it('illegal_move: la jugada responde 400 illegal_move', async () => {
    await expect(firstValueFrom(fakeApi('illegal_move').move('l_0001', 3))).rejects.toMatchObject({
      status: 400,
      error: { ok: false, error: 'illegal_move' },
    });
  });

  it('rechaza una columna que no está en legal_moves', async () => {
    await expect(firstValueFrom(fakeApi('playing').move('l_0001', 8))).rejects.toMatchObject({
      status: 400,
      error: { error: 'illegal_move' },
    });
  });

  it('rechaza jugar en una partida terminada con 409 game_over', async () => {
    await expect(firstValueFrom(fakeApi('won').move('l_0002', 0))).rejects.toMatchObject({
      status: 409,
      error: { error: 'game_over' },
    });
  });

  it('una jugada legal responde después de simular que el bot piensa', async () => {
    vi.useFakeTimers();
    let answered = false;
    fakeApi('playing').move('l_0001', 0).subscribe(() => (answered = true));

    await vi.advanceTimersByTimeAsync(100);
    expect(answered).toBe(false);
    await vi.advanceTimersByTimeAsync(1000);
    expect(answered).toBe(true);
  });
});
