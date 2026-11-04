import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { firstValueFrom } from 'rxjs';

import { fakeState } from './fake-scenarios';
import { API_URL, apiUrlFor } from './game-api';
import { GameApiService } from './game-api.service';

const BASE = 'http://bot.test:3000';

describe('GameApiService', () => {
  let api: GameApiService;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        GameApiService,
        { provide: API_URL, useValue: BASE },
      ],
    });
    api = TestBed.inject(GameApiService);
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  it('crea una partida con POST /local/games y devuelve el GameState', async () => {
    const response = firstValueFrom(api.create('bot'));

    const req = http.expectOne(`${BASE}/local/games`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ first: 'bot' });
    req.flush(fakeState('playing'));

    expect(await response).toEqual(fakeState('playing'));
  });

  it('manda la dificultad solo cuando se elige', async () => {
    const response = firstValueFrom(api.create('human', 'easy'));

    const req = http.expectOne(`${BASE}/local/games`);
    expect(req.request.body).toEqual({ first: 'human', difficulty: 'easy' });
    req.flush(fakeState('playing'));
    await response;
  });

  it('juega con POST /local/games/{id}/moves', async () => {
    const response = firstValueFrom(api.move('l_0001', 3));

    const req = http.expectOne(`${BASE}/local/games/l_0001/moves`);
    expect(req.request.method).toBe('POST');
    expect(req.request.body).toEqual({ column: 3 });
    req.flush(fakeState('playing'));
    await response;
  });

  it('consulta el estado con GET /local/games/{id}', async () => {
    const response = firstValueFrom(api.get('l_0001'));

    const req = http.expectOne(`${BASE}/local/games/l_0001`);
    expect(req.request.method).toBe('GET');
    req.flush(fakeState('playing'));
    await response;
  });

  it('propaga el error del contrato tal como lo manda el bot', async () => {
    const response = firstValueFrom(api.move('l_0001', 3));

    http
      .expectOne(`${BASE}/local/games/l_0001/moves`)
      .flush(
        { ok: false, error: 'illegal_move', message: 'La columna 3 está llena.' },
        { status: 400, statusText: 'Bad Request' },
      );

    await expect(response).rejects.toMatchObject({
      status: 400,
      error: { ok: false, error: 'illegal_move' },
    });
  });
});

describe('apiUrlFor', () => {
  it('usa el host de la página para que funcione desde la red local', () => {
    expect(apiUrlFor({ protocol: 'http:', hostname: '192.168.0.20' }, 3000)).toBe(
      'http://192.168.0.20:3000',
    );
    expect(apiUrlFor({ protocol: 'http:', hostname: 'localhost' }, 3000)).toBe(
      'http://localhost:3000',
    );
  });
});
