import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';

import {
  FAKE_SCENARIO,
  FAKE_THINK_MS,
  FakeGameApiService,
  fakeRequested,
} from './fake-game-api.service';
import { FakeScenario } from './fake-scenarios';

/** El fake con una demora fija de 600 ms, independiente del valor por defecto. */
function fakeApi(scenario: FakeScenario): FakeGameApiService {
  TestBed.configureTestingModule({
    providers: [
      FakeGameApiService,
      { provide: FAKE_SCENARIO, useValue: scenario },
      { provide: FAKE_THINK_MS, useValue: 600 },
    ],
  });
  return TestBed.inject(FakeGameApiService);
}

describe('FakeGameApiService', () => {
  afterEach(() => vi.useRealTimers());

  it('playing, abre el humano: tablero vacío y le toca al humano (1)', async () => {
    const state = await firstValueFrom(fakeApi('playing').create('human'));

    expect(state.board.flat().every((cell) => cell === 0)).toBe(true);
    expect([state.human, state.bot, state.to_move]).toEqual([1, 2, 1]);
    expect(state.last_bot).toBeNull();
  });

  it('playing, abre el bot: ya trae su apertura y le toca al humano (2)', async () => {
    const state = await firstValueFrom(fakeApi('playing').create('bot'));

    expect([state.human, state.bot, state.to_move]).toEqual([2, 1, 2]);
    expect(state.history).toEqual([3]);
    expect(state.board[0][3]).toBe(1);
    expect(state.last_bot?.column).toBe(3);
  });

  it('fixture: crea la partida de ejemplo del contrato', async () => {
    const state = await firstValueFrom(fakeApi('fixture').create('human'));

    expect(state.id).toBe('l_0001');
    expect(state.history).toEqual([3, 4, 3, 4, 3, 3]);
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

  it('la ficha del humano cae abajo, el bot responde en un orden fijo y las jugadas se acumulan', async () => {
    vi.useFakeTimers();
    const api = fakeApi('playing');
    await firstValueFrom(api.create('human'));

    const first = firstValueFrom(api.move('l_fake', 0));
    await vi.advanceTimersByTimeAsync(1000);
    const second = firstValueFrom(api.move('l_fake', 0));
    await vi.advanceTimersByTimeAsync(1000);
    const state = await second;

    expect((await first).board[0][0]).toBe(1);
    expect(state.board[1][0]).toBe(1);
    // El bot falso juega la primera columna disponible de 3, 4, 2, 5, 1, 6, 0, 7.
    expect(state.board[0][3]).toBe(2);
    expect(state.board[1][3]).toBe(2);
    expect(state.history).toEqual([0, 3, 0, 3]);
    expect(state.to_move).toBe(1);
    expect(state.last_bot?.column).toBe(3);
  });

  it('offline: la primera jugada se aplica pero la respuesta se pierde', async () => {
    vi.useFakeTimers();
    const api = fakeApi('offline');
    await firstValueFrom(api.create('human'));

    const lost = firstValueFrom(api.move('l_fake', 5));
    const failed = expect(lost).rejects.toMatchObject({ status: 0 });
    await vi.advanceTimersByTimeAsync(1000);
    await failed;

    const state = await firstValueFrom(api.get('l_fake'));
    expect(state.history).toEqual([5, 3]);
  });

  it('lost: las jugadas responden 404 unknown_game', async () => {
    const api = fakeApi('lost');
    await firstValueFrom(api.create('human'));

    await expect(firstValueFrom(api.move('l_fake', 0))).rejects.toMatchObject({
      status: 404,
      error: { error: 'unknown_game' },
    });
  });
});

describe('fakeRequested', () => {
  it('la app usa la API real salvo que la URL pida un escenario', () => {
    expect(fakeRequested('')).toBe(false);
    expect(fakeRequested('?foo=1')).toBe(false);
    expect(fakeRequested('?scenario=won')).toBe(true);
  });
});
