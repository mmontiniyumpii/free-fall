import { TestBed } from '@angular/core/testing';

import { FirstPlayer } from '../game/game-state';
import { Toolbar, ToolbarMode } from './toolbar';

function render(mode: ToolbarMode, first: FirstPlayer = 'human', busy = false) {
  const fixture = TestBed.createComponent(Toolbar);
  fixture.componentRef.setInput('mode', mode);
  fixture.componentRef.setInput('first', first);
  fixture.componentRef.setInput('busy', busy);
  const events: string[] = [];
  fixture.componentInstance.firstChange.subscribe((value) => events.push(`first:${value}`));
  fixture.componentInstance.newGame.subscribe(() => events.push('newGame'));
  fixture.componentInstance.backToSetup.subscribe(() => events.push('backToSetup'));
  fixture.componentInstance.retry.subscribe(() => events.push('retry'));
  fixture.detectChanges();
  const host = fixture.nativeElement as HTMLElement;
  const button = (text: string) =>
    Array.from(host.querySelectorAll('button')).find((b) => b.textContent?.includes(text)) ?? null;
  return { fixture, host, events, button };
}

describe('Toolbar', () => {
  it('en el inicio marca quién empieza y deja elegir al bot', () => {
    const { button, events } = render('setup', 'human');

    expect(button('Vos')?.getAttribute('aria-pressed')).toBe('true');
    expect(button('El bot')?.getAttribute('aria-pressed')).toBe('false');
    button('El bot')?.click();
    expect(events).toEqual(['first:bot']);
  });

  it('"Empezar partida" pide una partida nueva', () => {
    const { button, events } = render('setup');

    button('Empezar partida')?.click();

    expect(events).toEqual(['newGame']);
  });

  it('durante la partida, "Reiniciar" pide una partida nueva', () => {
    const { button, events } = render('playing');

    expect(button('Empezar partida')).toBeNull();
    button('Reiniciar')?.click();

    expect(events).toEqual(['newGame']);
  });

  it('al terminar, deja jugar de nuevo o volver a la configuración', () => {
    const { button, events } = render('over');

    button('Jugar de nuevo')?.click();
    button('Cambiar configuración')?.click();

    expect(events).toEqual(['newGame', 'backToSetup']);
  });

  it('con un pedido en curso las acciones quedan deshabilitadas', () => {
    const { button } = render('playing', 'human', true);

    expect(button('Reiniciar')?.disabled).toBe(true);
  });

  it('la ayuda de teclas aparece solo cuando se pide', () => {
    const { fixture, host } = render('playing');
    expect(host.querySelector('.hint')).toBeNull();

    fixture.componentRef.setInput('showKeys', true);
    fixture.detectChanges();

    expect(host.querySelector('.hint')?.textContent).toContain('teclas 1 a 8');
  });

  it('sin conexión ofrece reintentar ya o reiniciar', () => {
    const { button, events } = render('offline');

    button('Reintentar')?.click();
    button('Reiniciar')?.click();

    expect(events).toEqual(['retry', 'newGame']);
  });
});
