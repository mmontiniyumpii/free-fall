import { TestBed } from '@angular/core/testing';

import { App, RETRY_SECONDS } from './app';
import { FAKE_SCENARIO, FAKE_THINK_MS, FakeGameApiService } from './game/fake-game-api.service';
import { FakeScenario } from './game/fake-scenarios';
import { GameApi } from './game/game-api';

async function start(scenario: FakeScenario = 'playing') {
  TestBed.configureTestingModule({
    imports: [App],
    providers: [
      { provide: GameApi, useClass: FakeGameApiService },
      { provide: FAKE_SCENARIO, useValue: scenario },
      { provide: FAKE_THINK_MS, useValue: 0 },
      { provide: RETRY_SECONDS, useValue: 1 },
    ],
  });
  const fixture = TestBed.createComponent(App);
  await fixture.whenStable();
  const host = fixture.nativeElement as HTMLElement;
  const button = (text: string) =>
    Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) ?? null;
  /** Deja correr la respuesta del fake (delay 0) y el render. */
  const settle = async () => {
    await new Promise((resolve) => setTimeout(resolve));
    await fixture.whenStable();
  };
  const click = async (text: string) => {
    const target = button(text);
    if (!target) throw new Error(`No hay botón "${text}"`);
    target.click();
    await settle();
  };
  return {
    fixture,
    host,
    button,
    click,
    settle,
    title: () => host.querySelector('.title')?.textContent?.trim(),
    discs: (kind: 'human' | 'bot') => host.querySelectorAll(`app-board .disc--${kind}`).length,
    columns: () => host.querySelectorAll<HTMLElement>('app-board .column'),
  };
}

describe('App', () => {
  it('arranca en la pantalla de inicio con el tablero vacío', async () => {
    const app = await start();

    expect(app.title()).toBe('Cuatro en línea');
    expect(app.button('Empezar partida')).not.toBeNull();
    expect(app.host.querySelectorAll('app-board .cell').length).toBe(56);
    expect(app.host.querySelector('app-board .disc')).toBeNull();
  });

  it('empezar como humano: tablero vacío y te toca a vos', async () => {
    const app = await start();

    await app.click('Empezar partida');

    expect(app.title()).toBe('Tu turno');
    expect(app.host.querySelector('app-board .disc')).toBeNull();
    expect(app.button('Reiniciar')).not.toBeNull();
  });

  it('empezar como bot: aparece su apertura abajo y te toca a vos', async () => {
    const app = await start();

    await app.click('El bot');
    await app.click('Empezar partida');

    expect(app.title()).toBe('Tu turno');
    expect(app.discs('bot')).toBe(1);
    const column3 = app.columns()[3].querySelectorAll('.cell');
    expect(column3[column3.length - 1].querySelector('.disc--bot')).not.toBeNull();
  });

  it('al hacer clic en una columna la ficha aparece abajo', async () => {
    const app = await start();
    await app.click('Empezar partida');

    app.columns()[0].click();
    await app.settle();

    const cells = app.columns()[0].querySelectorAll('.cell');
    expect(cells[cells.length - 1].querySelector('.disc--human')).not.toBeNull();
  });

  it('"Reiniciar" vuelve a una partida nueva con el mismo inicio, sin pasar por la configuración', async () => {
    const app = await start();
    await app.click('El bot');
    await app.click('Empezar partida');
    app.columns()[0].click();
    await app.settle();
    expect(app.discs('human')).toBe(1);

    await app.click('Reiniciar');

    expect(app.discs('human')).toBe(0);
    expect(app.discs('bot')).toBe(1);
    expect(app.button('Empezar partida')).toBeNull();
  });

  it('al terminar la partida se puede volver a la configuración', async () => {
    const app = await start('won');
    await app.click('Empezar partida');
    expect(app.title()).toBe('Ganaste');

    await app.click('Cambiar configuración');

    expect(app.title()).toBe('Cuatro en línea');
    expect(app.button('Empezar partida')).not.toBeNull();
  });

  describe('errores del bot', () => {
    it('sin conexión: avisa, y al reintentar no juega dos veces la misma ficha', async () => {
      const app = await start('offline');
      await app.click('Empezar partida');

      app.columns()[5].click();
      await app.settle();

      expect(app.title()).toBe('Sin conexión');
      expect(app.host.querySelector('.board--offline')).not.toBeNull();
      // La ficha enviada sigue a la vista mientras no hay respuesta.
      expect(app.discs('human')).toBe(1);

      await app.click('Reintentar');

      // El fake aplicó la jugada antes de perder la respuesta: el reintento la
      // encuentra en history y no la reenvía.
      expect(app.title()).toBe('Tu turno');
      expect(app.discs('human')).toBe(1);
      expect(app.discs('bot')).toBe(1);
      expect(app.host.querySelector('.board--offline')).toBeNull();
    });

    it('sin conexión reintenta solo al terminar la cuenta regresiva', async () => {
      const app = await start('offline');
      await app.click('Empezar partida');
      app.columns()[5].click();
      await app.settle();
      expect(app.host.querySelector('.detail')?.textContent).toContain('Reintentamos en 1 s');

      await new Promise((resolve) => setTimeout(resolve, 1100));
      await app.settle();

      expect(app.title()).toBe('Tu turno');
      expect(app.discs('human')).toBe(1);
    });

    it('columna llena según el bot: se descarta la ficha y se avisa', async () => {
      const app = await start('illegal_move');
      await app.click('Empezar partida');
      const before = app.discs('human');

      app.columns()[5].click();
      await app.settle();

      expect(app.discs('human')).toBe(before);
      expect(app.host.querySelector('.detail')?.textContent).toBe(
        'La columna 6 está llena. Elegí otra.',
      );
    });

    it('partida perdida (404): lo explica y ofrece empezar otra', async () => {
      const app = await start('lost');
      await app.click('Empezar partida');

      app.columns()[0].click();
      await app.settle();

      expect(app.title()).toBe('La partida ya no existe');
      expect(app.discs('human')).toBe(0);
      expect(app.button('Jugar de nuevo')).not.toBeNull();
    });
  });

  it('perder: muestra al bot como ganador, la línea resaltada y la opción de jugar de nuevo', async () => {
    const app = await start('bot_won');
    await app.click('Empezar partida');

    expect(app.title()).toBe('Ganó el bot');
    expect(app.host.querySelectorAll('app-board .ring').length).toBe(4);
    expect(app.host.querySelector('app-board .stroke')).not.toBeNull();
    expect(app.button('Jugar de nuevo')).not.toBeNull();
  });
});
